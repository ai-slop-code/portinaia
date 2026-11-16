package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

const stateVersion = 1

// lockSnapshotState makes a process the exclusive owner of pending snapshot
// replacement and acknowledgement for this credential path.
func lockSnapshotState(ctx context.Context, statePath string) (*os.File, error) {
	dir := filepath.Dir(statePath)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create state directory: %w", err)
	}
	path := statePath + ".lock"
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|syscall.O_NOFOLLOW, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open state owner lock: %w", err)
	}
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 || validateOwner(info) != nil {
		file.Close()
		return nil, errors.New("state owner lock must be a private regular file owned by the current user")
	}
	for {
		err = syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			return file, nil
		}
		if err != syscall.EWOULDBLOCK && err != syscall.EAGAIN {
			file.Close()
			return nil, fmt.Errorf("lock pending state: %w", err)
		}
		select {
		case <-ctx.Done():
			file.Close()
			return nil, ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
	}
}

type CredentialState struct {
	Version              int    `json:"version"`
	ServerURL            string `json:"server_url"`
	AllowInsecureHTTP    bool   `json:"allow_insecure_http,omitempty"`
	AgentID              string `json:"agent_id"`
	ComputeID            string `json:"compute_id"`
	Credential           string `json:"credential"`
	CredentialGeneration int64  `json:"credential_generation"`
	ProtocolVersion      int    `json:"protocol_version"`
	ClientVersion        string `json:"client_version"`
}

func LoadState(path string) (CredentialState, error) {
	var state CredentialState
	file, err := openPrivateFile(path)
	if err != nil {
		return state, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, 16*1024+1))
	if err != nil {
		return state, err
	}
	if len(data) > 16*1024 {
		return state, errors.New("credential state exceeds size limit")
	}
	var members map[string]json.RawMessage
	if err := json.Unmarshal(data, &members); err != nil {
		return state, fmt.Errorf("decode agent credential state: %w", err)
	}
	if err := requireMembers(members, "version", "server_url", "agent_id", "compute_id", "credential", "credential_generation", "protocol_version", "client_version"); err != nil {
		return state, fmt.Errorf("decode agent credential state: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&state); err != nil {
		return state, fmt.Errorf("decode agent credential state: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return CredentialState{}, errors.New("credential state contains trailing data")
	}
	if err := state.Validate(); err != nil {
		return CredentialState{}, err
	}
	return state, nil
}

func SaveState(path string, state CredentialState) error {
	state.Version = stateVersion
	if err := state.Validate(); err != nil {
		return err
	}
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return fmt.Errorf("create state directory: %w", err)
	}
	info, err := os.Lstat(directory)
	if err != nil {
		return fmt.Errorf("inspect state directory: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return errors.New("state directory must be a real directory")
	}
	if info.Mode().Perm()&0o077 != 0 {
		return errors.New("state directory permissions must exclude group and other access")
	}
	if err := validateOwner(info); err != nil {
		return err
	}
	if _, err := os.Lstat(path); err == nil {
		file, err := openPrivateFile(path)
		if err != nil {
			return err
		}
		if err := file.Close(); err != nil {
			return err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect state file: %w", err)
	}
	data, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("encode agent credential state: %w", err)
	}
	temporary, err := os.CreateTemp(directory, ".credential-state-*")
	if err != nil {
		return fmt.Errorf("create temporary state file: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return fmt.Errorf("restrict temporary state file: %w", err)
	}
	if _, err := temporary.Write(data); err != nil {
		temporary.Close()
		return fmt.Errorf("write temporary state file: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return fmt.Errorf("sync temporary state file: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close temporary state file: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("replace credential state: %w", err)
	}
	directoryFile, err := os.Open(directory)
	if err != nil {
		return fmt.Errorf("open state directory for sync: %w", err)
	}
	defer directoryFile.Close()
	if err := directoryFile.Sync(); err != nil {
		return fmt.Errorf("sync state directory: %w", err)
	}
	return nil
}

func LoadPendingSnapshot(statePath string) ([]byte, error) {
	path := statePath + ".pending"
	file, err := openPrivateFile(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, 32<<20))
	if err != nil {
		return nil, err
	}
	if len(data) == 0 || len(data) >= 32<<20 {
		return nil, errors.New("pending snapshot is empty or exceeds the size limit")
	}
	if err := validatePendingReport(data); err != nil {
		return nil, errors.New("pending snapshot state is invalid")
	}
	return data, nil
}

func SavePendingSnapshot(statePath string, data []byte) error {
	if len(data) == 0 || len(data) >= 32<<20 {
		return errors.New("pending snapshot is empty or exceeds the size limit")
	}
	if err := validatePendingReport(data); err != nil {
		return fmt.Errorf("pending snapshot is invalid: %w", err)
	}
	path := statePath + ".pending"
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return err
	}
	info, err := os.Lstat(directory)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o077 != 0 {
		return errors.New("pending snapshot directory must be private")
	}
	if err := validateOwner(info); err != nil {
		return err
	}
	if _, err := os.Lstat(path); err == nil {
		existing, openErr := openPrivateFile(path)
		if openErr != nil {
			return openErr
		}
		existing.Close()
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	temp, err := os.CreateTemp(directory, ".pending-snapshot-*")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if err := temp.Chmod(0o600); err != nil {
		temp.Close()
		return err
	}
	if _, err := temp.Write(data); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tempPath, path); err != nil {
		return err
	}
	dir, err := os.Open(directory)
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}

func ClearPendingSnapshot(statePath string) error {
	path := statePath + ".pending"
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	dir, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}

// ClearPendingSnapshotIfMatch acknowledges only the report generation that was
// actually uploaded. A stale in-flight response can never delete newer state.
func ClearPendingSnapshotIfMatch(statePath, reportID string) error {
	current, err := LoadPendingSnapshot(statePath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var marker struct {
		ReportID string `json:"report_id"`
	}
	if err := json.Unmarshal(current, &marker); err != nil {
		return err
	}
	if marker.ReportID != reportID {
		return nil
	}
	return ClearPendingSnapshot(statePath)
}

func validatePendingReport(data []byte) error {
	var members map[string]json.RawMessage
	if err := json.Unmarshal(data, &members); err != nil {
		return err
	}
	if err := requireMembers(members, "report_id", "protocol_version", "client_version", "collection_started_at", "collection_finished_at"); err != nil {
		return err
	}
	for section, payload := range members {
		if section != "host" && section != "disks" && section != "interfaces" && section != "filesystems" && section != "podman" {
			continue
		}
		if section == "podman" {
			var items []json.RawMessage
			if err := json.Unmarshal(payload, &items); err != nil {
				return fmt.Errorf("pending %s section must be an array", section)
			}
			for _, item := range items {
				if err := validateCollectorUnion(item, "containers"); err != nil {
					return fmt.Errorf("pending podman union: %w", err)
				}
			}
			continue
		}
		dataMember := map[string]string{"host": "host", "disks": "disks", "interfaces": "interfaces", "filesystems": "filesystems"}[section]
		if err := validateCollectorUnion(payload, dataMember); err != nil {
			return fmt.Errorf("pending %s union: %w", section, err)
		}
	}
	var report struct {
		ReportID        string `json:"report_id"`
		ProtocolVersion int    `json:"protocol_version"`
		ClientVersion   string `json:"client_version"`
		Started         string `json:"collection_started_at"`
		Finished        string `json:"collection_finished_at"`
	}
	decoder := json.NewDecoder(io.LimitReader(bytes.NewReader(data), 32<<20))
	if err := decoder.Decode(&report); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return errors.New("trailing JSON")
	}
	if !validUUID(report.ReportID) || report.ProtocolVersion != protocolVersion || report.ClientVersion == "" || !validServerTime(report.Started) || !validServerTime(report.Finished) {
		return errors.New("incomplete report envelope")
	}
	return nil
}

func validateCollectorUnion(raw json.RawMessage, valueMember string) error {
	var members map[string]json.RawMessage
	if err := json.Unmarshal(raw, &members); err != nil {
		return err
	}
	if err := requireMembers(members, "status"); err != nil {
		return err
	}
	var status string
	if err := json.Unmarshal(members["status"], &status); err != nil {
		return errors.New("collector status must be a string")
	}
	switch status {
	case "success":
		if err := requireMembers(members, valueMember); err != nil {
			return err
		}
		if _, exists := members["error"]; exists {
			return errors.New("successful collector cannot contain error")
		}
	case "failed":
		if err := requireMembers(members, "error"); err != nil {
			return err
		}
		if _, exists := members[valueMember]; exists {
			return errors.New("failed collector cannot contain success value")
		}
	default:
		return errors.New("unknown collector status")
	}
	return nil
}

func requireMembers(members map[string]json.RawMessage, names ...string) error {
	if members == nil {
		return errors.New("document must be a JSON object")
	}
	for _, name := range names {
		value, ok := members[name]
		if !ok || string(value) == "null" {
			return fmt.Errorf("required member %q is missing or null", name)
		}
	}
	return nil
}

func (state CredentialState) Validate() error {
	if state.Version != stateVersion {
		return fmt.Errorf("unsupported agent state version %d", state.Version)
	}
	if state.ServerURL == "" || state.AgentID == "" || state.ComputeID == "" || state.Credential == "" || state.ClientVersion == "" || state.ProtocolVersion != 1 || state.CredentialGeneration < 1 {
		return errors.New("agent credential state is incomplete")
	}
	return nil
}

func openPrivateFile(path string) (*os.File, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("inspect credential state: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, errors.New("credential state must be a regular file, not a symlink")
	}
	if info.Mode().Perm()&0o077 != 0 {
		return nil, errors.New("credential state permissions must be 0600")
	}
	if err := validateOwner(info); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return nil, fmt.Errorf("open credential state without following links: %w", err)
	}
	openedInfo, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, err
	}
	if err := validateOwner(openedInfo); err != nil {
		file.Close()
		return nil, err
	}
	if openedInfo.Mode().Perm()&0o077 != 0 {
		file.Close()
		return nil, errors.New("credential state permissions must be 0600")
	}
	return file, nil
}

func validateOwner(info os.FileInfo) error {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return errors.New("cannot verify credential state owner")
	}
	if stat.Uid != uint32(os.Getuid()) {
		return errors.New("credential state must be owned by the current user")
	}
	return nil
}
