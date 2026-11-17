package server

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/argon2"
	apigen "portinaia/internal/api/generated"
	"portinaia/internal/identity"
)

const sessionCookie = "portinaia_session"
const sessionLifetime = 12 * time.Hour
const lastSeenInterval = time.Minute

var argonSlots = make(chan struct{}, 4)
var loginAttempts = newLoginAttemptLimiter(4096, 5, 15*time.Minute)

type sessionInfo struct {
	AdministratorID string `json:"administrator_id"`
	Username        string `json:"username"`
	CreatedAt       string `json:"created_at"`
	ExpiresAt       string `json:"expires_at"`
	LastSeenAt      string `json:"last_seen_at"`
	CSRFToken       string `json:"csrf_token"`
}

func bootstrapAdministrator(db *sql.DB, username, password string) error {
	username = strings.TrimSpace(username)
	if err := validateAdministratorCredentials(username, password); err != nil {
		return err
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var count int
	if err := tx.QueryRow(`SELECT count(*) FROM administrators`).Scan(&count); err != nil {
		return err
	}
	if count != 0 {
		return fmt.Errorf("administrator already exists")
	}
	id, err := randomID()
	if err != nil {
		return err
	}
	hash, params, err := derivePasswordHash(password)
	if err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err = tx.Exec(`INSERT INTO administrators (id, username, password_hash, password_algorithm, password_parameters, created_at, updated_at) VALUES (?, ?, ?, 'argon2id', ?, ?, ?)`, id, username, hash, params, now, now)
	if err != nil {
		return err
	}
	if err := appendAudit(tx, auditEntry{OccurredAt: now, ActorType: "system", ActorID: systemActorID, Action: "administrator.bootstrap", ResourceType: "administrator", ResourceID: id, After: map[string]any{"username": username}, Result: "success"}); err != nil {
		return err
	}
	return tx.Commit()
}

// BootstrapAdministrator creates the sole initial administrator, only on an empty database.
func BootstrapAdministrator(db *sql.DB, username, password string) error {
	return bootstrapAdministrator(db, username, password)
}

// ResetAdministratorPassword changes the password and revokes all browser sessions transactionally.
func ResetAdministratorPassword(db *sql.DB, username, password string) error {
	username = strings.TrimSpace(username)
	if err := validateAdministratorCredentials(username, password); err != nil {
		return err
	}
	hash, params, err := derivePasswordHash(password)
	if err != nil {
		return err
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var id string
	err = tx.QueryRow(`SELECT id FROM administrators WHERE username=? COLLATE NOCASE AND disabled_at IS NULL`, username).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("enabled administrator %q not found", username)
	}
	if err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	result, err := tx.Exec(`UPDATE administrators SET password_hash=?,password_algorithm='argon2id',password_parameters=?,updated_at=? WHERE id=? AND disabled_at IS NULL`, hash, params, now, id)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n != 1 {
		return fmt.Errorf("administrator password was not updated")
	}
	sessions, err := tx.Exec(`UPDATE sessions SET revoked_at=? WHERE administrator_id=? AND revoked_at IS NULL`, now, id)
	if err != nil {
		return err
	}
	revoked, _ := sessions.RowsAffected()
	if err = appendAudit(tx, auditEntry{OccurredAt: now, ActorType: "system", ActorID: systemActorID, Action: "administrator.password_reset", ResourceType: "administrator", ResourceID: id, After: map[string]any{"username": username, "sessions_revoked": revoked}, Result: "success"}); err != nil {
		return err
	}
	return tx.Commit()
}

func validateAdministratorCredentials(username, password string) error {
	if len(username) == 0 || len(username) > 128 || len(password) < 12 || len(password) > 1024 {
		return fmt.Errorf("username must be 1–128 characters and password 12–1024 characters")
	}
	return nil
}

func derivePasswordHash(password string) (string, string, error) {
	argonSlots <- struct{}{}
	defer func() { <-argonSlots }()
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", "", err
	}
	parameters := `{"memory_kib":19456,"iterations":2,"parallelism":1,"salt":"` + base64.RawStdEncoding.EncodeToString(salt) + `"}`
	hash := argon2.IDKey([]byte(password), salt, 2, 19456, 1, 32)
	return base64.RawStdEncoding.EncodeToString(hash), parameters, nil
}

func loginHandler(db *sql.DB, secureCookie bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if !sameOrigin(r) || r.Header.Get("Authorization") != "" {
			writeProblemForRequest(w, r, http.StatusForbidden, "forbidden", "login requires same-origin browser credentials")
			return
		}
		key := clientAddress(r)
		if !loginAttempts.allow(key, time.Now()) {
			w.Header().Set("Retry-After", "900")
			writeProblemForRequest(w, r, http.StatusTooManyRequests, "rate_limited", "too many login attempts")
			return
		}
		var input apigen.LoginRequest
		if err := decodeJSON(w, r, 4096, &input); err != nil {
			writeJSONDecodeError(w, r, err)
			return
		}
		if input.Password == nil || input.Username == "" || *input.Password == "" {
			writeProblemForRequest(w, r, http.StatusBadRequest, "invalid_request", "username and password are required")
			return
		}
		var id, username, stored, params string
		err := db.QueryRowContext(r.Context(), `SELECT id, username, password_hash, password_parameters FROM administrators WHERE username=? COLLATE NOCASE AND disabled_at IS NULL`, strings.TrimSpace(input.Username)).Scan(&id, &username, &stored, &params)
		if errors.Is(err, sql.ErrNoRows) {
			stored, params, _ = dummyPasswordHash()
			err = nil
		}
		var p struct {
			Memory      uint32 `json:"memory_kib"`
			Iterations  uint32 `json:"iterations"`
			Parallelism uint8  `json:"parallelism"`
			Salt        string `json:"salt"`
		}
		parseErr := json.Unmarshal([]byte(params), &p)
		salt, saltErr := base64.RawStdEncoding.DecodeString(p.Salt)
		ok := false
		if err == nil && parseErr == nil && saltErr == nil && p.Memory >= 8192 && p.Memory <= 262144 && p.Iterations > 0 && p.Iterations <= 10 && p.Parallelism > 0 {
			if err := acquireArgonSlot(r.Context(), argonSlots); err != nil {
				writeProblemForRequest(w, r, http.StatusRequestTimeout, "request_cancelled", "login request was cancelled")
				return
			}
			got := argon2.IDKey([]byte(*input.Password), salt, p.Iterations, p.Memory, p.Parallelism, 32)
			<-argonSlots
			want, decodeErr := base64.RawStdEncoding.DecodeString(stored)
			ok = decodeErr == nil && constantEqual(got, want)
		}
		if !ok {
			loginAttempts.fail(key, time.Now())
			http.Error(w, "invalid username or password", 401)
			return
		}
		loginAttempts.success(key)
		token, err := randomSecret(32)
		if err != nil {
			http.Error(w, "internal error", 500)
			return
		}
		csrf := csrfFor(token)
		sid, err := randomID()
		if err != nil {
			http.Error(w, "internal error", 500)
			return
		}
		now := time.Now().UTC()
		created, expires := now.Format(time.RFC3339), now.Add(sessionLifetime).Format(time.RFC3339)
		_, err = db.ExecContext(r.Context(), `INSERT INTO sessions (id, administrator_id, credential_hash, csrf_hash, created_at, expires_at, last_seen_at) VALUES (?, ?, ?, ?, ?, ?, ?)`, sid, id, hashSecret(token), hashSecret(csrf), created, expires, created)
		if err != nil {
			http.Error(w, "internal error", 500)
			return
		}
		http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: token, Path: "/", HttpOnly: true, Secure: secureCookie, SameSite: http.SameSiteStrictMode, Expires: now.Add(sessionLifetime)})
		writeJSON(w, 200, sessionInfo{id, username, created, expires, created, csrf})
	}
}

// acquireArgonSlot waits for bounded password-hash capacity without ignoring cancellation.
func acquireArgonSlot(ctx context.Context, slots chan struct{}) error {
	select {
	case slots <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func sessionHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		s, ok := authenticate(db, r)
		if !ok {
			http.Error(w, "unauthorized", 401)
			return
		}
		writeJSON(w, 200, s)
	}
}

func logoutHandler(db *sql.DB, secureCookie bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		s, ok := authenticate(db, r)
		if !ok {
			http.Error(w, "unauthorized", 401)
			return
		}
		if !constantEqual([]byte(r.Header.Get("X-CSRF-Token")), []byte(s.CSRFToken)) {
			http.Error(w, "forbidden", 403)
			return
		}
		cookie, _ := r.Cookie(sessionCookie)
		result, err := db.ExecContext(r.Context(), `UPDATE sessions SET revoked_at=? WHERE credential_hash=? AND revoked_at IS NULL`, time.Now().UTC().Format(time.RFC3339), hashSecret(cookie.Value))
		if err != nil {
			writeProblemForRequest(w, r, http.StatusInternalServerError, "internal_error", "session revocation failed")
			return
		}
		if n, err := result.RowsAffected(); err != nil || n != 1 {
			writeProblemForRequest(w, r, http.StatusInternalServerError, "internal_error", "session revocation failed")
			return
		}
		http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", HttpOnly: true, Secure: secureCookie, SameSite: http.SameSiteStrictMode, MaxAge: -1})
		w.WriteHeader(http.StatusNoContent)
	}
}

func authenticate(db *sql.DB, r *http.Request) (sessionInfo, bool) {
	cookie, err := r.Cookie(sessionCookie)
	if err != nil {
		return sessionInfo{}, false
	}
	var s sessionInfo
	var csrfHash string
	err = db.QueryRowContext(r.Context(), `SELECT a.id,a.username,s.created_at,s.expires_at,s.last_seen_at,s.csrf_hash FROM sessions s JOIN administrators a ON a.id=s.administrator_id WHERE s.credential_hash=? AND s.revoked_at IS NULL AND s.expires_at>? AND a.disabled_at IS NULL`, hashSecret(cookie.Value), time.Now().UTC().Format(time.RFC3339)).Scan(&s.AdministratorID, &s.Username, &s.CreatedAt, &s.ExpiresAt, &s.LastSeenAt, &csrfHash)
	if err != nil {
		return sessionInfo{}, false
	}
	s.CSRFToken = csrfFor(cookie.Value)
	if !constantEqual([]byte(hashSecret(s.CSRFToken)), []byte(csrfHash)) {
		return sessionInfo{}, false
	}
	if last, parseErr := time.Parse(time.RFC3339, s.LastSeenAt); parseErr == nil && time.Since(last) >= lastSeenInterval {
		_, _ = db.ExecContext(r.Context(), `UPDATE sessions SET last_seen_at=? WHERE credential_hash=? AND last_seen_at<=?`, time.Now().UTC().Format(time.RFC3339), hashSecret(cookie.Value), time.Now().UTC().Add(-lastSeenInterval).Format(time.RFC3339))
	}
	return s, true
}

func sameOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return false
	}
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" || u.User != nil {
		return false
	}
	return strings.EqualFold(u.Host, r.Host) && (u.Scheme == "https" || u.Scheme == "http" && r.TLS == nil)
}

func clientAddress(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}

var dummyHashOnce sync.Once
var dummyHash, dummyParams string

func dummyPasswordHash() (string, string, error) {
	dummyHashOnce.Do(func() { dummyHash, dummyParams, _ = derivePasswordHash("portinaia-invalid-account-dummy-password") })
	return dummyHash, dummyParams, nil
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func randomSecret(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
func randomID() (string, error) { return identity.New() }
func hashSecret(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
func csrfFor(token string) string {
	mac := hmac.New(sha256.New, []byte(token))
	_, _ = mac.Write([]byte("portinaia csrf v1"))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
func constantEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	var x byte
	for i := range a {
		x |= a[i] ^ b[i]
	}
	return x == 0
}

type attemptWindow struct {
	start time.Time
	count int
}
type loginAttemptLimiter struct {
	mu         sync.Mutex
	entries    map[string]attemptWindow
	max, limit int
	window     time.Duration
}

func newLoginAttemptLimiter(max, limit int, window time.Duration) *loginAttemptLimiter {
	return &loginAttemptLimiter{entries: make(map[string]attemptWindow), max: max, limit: limit, window: window}
}
func (l *loginAttemptLimiter) allow(key string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	v, ok := l.entries[key]
	return !ok || now.Sub(v.start) >= l.window || v.count < l.limit
}
func (l *loginAttemptLimiter) fail(key string, now time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	v, ok := l.entries[key]
	if !ok || now.Sub(v.start) >= l.window {
		v = attemptWindow{start: now}
	}
	v.count++
	if len(l.entries) >= l.max {
		for k := range l.entries {
			delete(l.entries, k)
			break
		}
	}
	l.entries[key] = v
}
func (l *loginAttemptLimiter) success(key string) { l.mu.Lock(); delete(l.entries, key); l.mu.Unlock() }
