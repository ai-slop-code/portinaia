package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"portinaia/internal/agent"
	"portinaia/internal/logging"
	"portinaia/internal/version"
)

const defaultStatePath = "/var/lib/portinaia-agent/credential.json"

func main() {
	level, err := logging.LevelFromEnv()
	slog.SetDefault(logging.New(os.Stderr, level))
	if err != nil {
		slog.Error("configuration failed", "error", err)
		os.Exit(1)
	}
	if err := run(os.Args[1:]); err != nil {
		slog.Error("command failed", "error", err)
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
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return agent.Run(ctx, agent.RunConfig{StatePath: statePath, HeartbeatInterval: heartbeatInterval, SnapshotInterval: snapshotInterval, PodmanEndpoints: endpoints, Logger: slog.Default()})
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
