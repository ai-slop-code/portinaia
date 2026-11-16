package main

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"flag"
	"fmt"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"portinaia/internal/agent"
	"portinaia/internal/version"
)

const defaultStatePath = "/var/lib/portinaia-agent/credential.json"

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "portinaia-agent:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		usage()
		return fmt.Errorf("command is required")
	}
	switch args[0] {
	case "version":
		if len(args) != 1 {
			usage()
			return fmt.Errorf("version takes no arguments")
		}
		fmt.Printf("portinaia-agent %s\n", version.String())
		return nil
	case "enroll":
		flags := flag.NewFlagSet("enroll", flag.ContinueOnError)
		serverURL := flags.String("server", "", "Portinaia server base URL")
		tokenFile := flags.String("token-file", "", "private file containing the one-time enrollment token")
		statePath := flags.String("state", defaultStatePath, "private credential state file")
		clientVersion := flags.String("client-version", version.String(), "agent client version")
		allowInsecureHTTP := flags.Bool("allow-insecure-http", false, "allow HTTP for a trusted private/VPN endpoint; persisted in credential state")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if *serverURL == "" || *tokenFile == "" {
			return fmt.Errorf("enroll requires -server and -token-file")
		}
		var err error
		if *allowInsecureHTTP {
			err = agent.EnrollAllowInsecureHTTP(*serverURL, *tokenFile, *statePath, *clientVersion)
		} else {
			err = agent.Enroll(*serverURL, *tokenFile, *statePath, *clientVersion)
		}
		if err != nil {
			return err
		}
		fmt.Printf("enrolled; credential state saved to %s\n", filepath.Clean(*statePath))
		return nil
	case "heartbeat":
		flags := flag.NewFlagSet("heartbeat", flag.ContinueOnError)
		statePath := flags.String("state", defaultStatePath, "private credential state file")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if err := agent.Heartbeat(*statePath); err != nil {
			return err
		}
		fmt.Println("heartbeat accepted")
		return nil
	case "snapshot":
		flags := flag.NewFlagSet("snapshot", flag.ContinueOnError)
		statePath := flags.String("state", defaultStatePath, "private credential state file")
		var socketFlags repeatedStrings
		flags.Var(&socketFlags, "podman-socket", "explicit Podman endpoint NAME=UID:/absolute/socket (repeatable)")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		endpoints, err := parsePodmanEndpoints(socketFlags)
		if err != nil {
			return err
		}
		if err := agent.Snapshot(*statePath, endpoints...); err != nil {
			return err
		}
		fmt.Println("snapshot accepted")
		return nil
	case "run":
		flags := flag.NewFlagSet("run", flag.ContinueOnError)
		statePath := flags.String("state", defaultStatePath, "private credential state file")
		interval := flags.Duration("heartbeat-interval", time.Minute, "heartbeat interval")
		snapshotInterval := flags.Duration("snapshot-interval", 5*time.Minute, "snapshot collection interval")
		var socketFlags repeatedStrings
		flags.Var(&socketFlags, "podman-socket", "explicit Podman endpoint NAME=UID:/absolute/socket (repeatable)")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if *interval < 15*time.Second || *interval > 10*time.Minute {
			return fmt.Errorf("heartbeat interval must be between 15s and 10m")
		}
		if *snapshotInterval < time.Minute || *snapshotInterval > 24*time.Hour {
			return fmt.Errorf("snapshot interval must be between 1m and 24h")
		}
		endpoints, err := parsePodmanEndpoints(socketFlags)
		if err != nil {
			return err
		}
		return runAgent(*statePath, *interval, *snapshotInterval, endpoints)
	default:
		usage()
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func runAgent(statePath string, heartbeatInterval, snapshotInterval time.Duration, endpoints []agent.PodmanEndpoint) error {
	// These intervals are explicit local operator overrides; server schedules are
	// validated by the protocol client but do not silently rewrite local policy.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	results := make(chan error, 2)
	go func() { results <- runHeartbeats(ctx, statePath, heartbeatInterval) }()
	go func() { results <- runSnapshots(ctx, statePath, snapshotInterval, endpoints) }()
	err := <-results
	cancel()
	// Both workers must stop before run returns (and the service considers exit complete).
	<-results
	return err
}

func runHeartbeats(ctx context.Context, statePath string, interval time.Duration) error {
	if err := heartbeatWithRetry(ctx, statePath); err != nil {
		return err
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := heartbeatWithRetry(ctx, statePath); err != nil {
				return err
			}
		}
	}
}

func runSnapshots(ctx context.Context, statePath string, interval time.Duration, endpoints []agent.PodmanEndpoint) error {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	attempt := 0
	for {
		if err := agent.SnapshotContext(ctx, statePath, endpoints...); err != nil {
			var responseError *agent.HTTPError
			if errors.As(err, &responseError) && responseError.Permanent {
				return err
			}
			var networkError *url.Error
			if !errors.As(err, &responseError) && !errors.As(err, &networkError) {
				return fmt.Errorf("snapshot local/configuration failure: %w", err)
			}
			fmt.Fprintln(os.Stderr, "snapshot failed; recollection or upload will be retried:", err)
			delay := retryDelay(attempt)
			if errors.As(err, &responseError) && responseError.RetryAfter > delay {
				delay = responseError.RetryAfter
			}
			timer := time.NewTimer(delay)
			attempt++
			select {
			case <-ctx.Done():
				timer.Stop()
				return nil
			case <-timer.C:
			case <-ticker.C:
			}
			continue
		}
		attempt = 0
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func heartbeatWithRetry(ctx context.Context, statePath string) error {
	for attempt := 0; ; attempt++ {
		err := agent.HeartbeatContext(ctx, statePath)
		if err == nil {
			return nil
		}
		var responseError *agent.HTTPError
		if errors.As(err, &responseError) && responseError.Permanent {
			return err
		}
		fmt.Fprintln(os.Stderr, "heartbeat failed; retrying:", err)
		delay := retryDelay(attempt)
		if errors.As(err, &responseError) && responseError.RetryAfter > delay {
			delay = responseError.RetryAfter
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil
		case <-timer.C:
		}
	}
}

func retryDelay(attempt int) time.Duration {
	if attempt > 8 {
		attempt = 8
	}
	ceiling := time.Second * time.Duration(1<<attempt)
	var random [8]byte
	if _, err := rand.Read(random[:]); err != nil {
		return ceiling / 2
	}
	return time.Duration(binary.LittleEndian.Uint64(random[:]) % uint64(ceiling+1))
}

type repeatedStrings []string

func (values *repeatedStrings) String() string         { return strings.Join(*values, ",") }
func (values *repeatedStrings) Set(value string) error { *values = append(*values, value); return nil }

func parsePodmanEndpoints(values []string) ([]agent.PodmanEndpoint, error) {
	endpoints := make([]agent.PodmanEndpoint, 0, len(values))
	for _, value := range values {
		endpoint, err := agent.ParsePodmanEndpoint(value)
		if err != nil {
			return nil, err
		}
		endpoints = append(endpoints, endpoint)
	}
	if err := agent.ValidatePodmanEndpoints(endpoints); err != nil {
		return nil, err
	}
	return endpoints, nil
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: portinaia-agent {version|enroll -server URL -token-file PATH [-state PATH]|heartbeat [-state PATH]|snapshot [-state PATH] [-podman-socket NAME=UID:/socket]|run [-state PATH] [-heartbeat-interval 1m] [-snapshot-interval 5m] [-podman-socket NAME=UID:/socket]}")
}
