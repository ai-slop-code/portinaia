package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"portinaia/internal/database"
	"portinaia/internal/server"
	"portinaia/internal/version"
)

type config struct {
	listenAddress              string
	databasePath               string
	publicURL                  string
	trustedProxyCIDRs          string
	discoveryRetentionDays     int
	discoveryRetentionBatch    int
	discoveryRetentionInterval time.Duration
}

func loadConfig() config {
	cfg := config{listenAddress: ":8080", databasePath: "portinaia.db", publicURL: "http://127.0.0.1:8080", discoveryRetentionDays: 90, discoveryRetentionBatch: 1000, discoveryRetentionInterval: time.Hour}
	if value := os.Getenv("PORTINAIA_LISTEN"); value != "" {
		cfg.listenAddress = value
	}
	if value := os.Getenv("PORTINAIA_DB"); value != "" {
		cfg.databasePath = value
	}
	if value := os.Getenv("PORTINAIA_PUBLIC_URL"); value != "" {
		cfg.publicURL = value
	}
	if value := os.Getenv("PORTINAIA_TRUSTED_PROXY_CIDRS"); value != "" {
		cfg.trustedProxyCIDRs = value
	}
	if value := os.Getenv("PORTINAIA_DISCOVERY_RETENTION_DAYS"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil {
			cfg.discoveryRetentionDays = -1
		} else {
			cfg.discoveryRetentionDays = parsed
		}
	}
	if value := os.Getenv("PORTINAIA_DISCOVERY_RETENTION_BATCH"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil {
			cfg.discoveryRetentionBatch = -1
		} else {
			cfg.discoveryRetentionBatch = parsed
		}
	}
	if value := os.Getenv("PORTINAIA_DISCOVERY_RETENTION_INTERVAL"); value != "" {
		parsed, err := time.ParseDuration(value)
		if err != nil {
			cfg.discoveryRetentionInterval = -1
		} else {
			cfg.discoveryRetentionInterval = parsed
		}
	}
	return cfg
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		log.Print(err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		usage()
		return fmt.Errorf("command is required")
	}
	cfg := loadConfig()
	switch args[0] {
	case "server":
		flags := flag.NewFlagSet("server", flag.ContinueOnError)
		flags.StringVar(&cfg.listenAddress, "listen", cfg.listenAddress, "HTTP listen address")
		flags.StringVar(&cfg.databasePath, "db", cfg.databasePath, "SQLite database path")
		flags.StringVar(&cfg.publicURL, "public-url", cfg.publicURL, "externally visible base URL")
		flags.StringVar(&cfg.trustedProxyCIDRs, "trusted-proxy-cidrs", cfg.trustedProxyCIDRs, "comma-separated trusted reverse-proxy CIDRs")
		flags.IntVar(&cfg.discoveryRetentionDays, "discovery-retention-days", cfg.discoveryRetentionDays, "accepted discovery report retention age in days")
		flags.IntVar(&cfg.discoveryRetentionBatch, "discovery-retention-batch", cfg.discoveryRetentionBatch, "maximum discovery reports/content deleted per retention batch")
		flags.DurationVar(&cfg.discoveryRetentionInterval, "discovery-retention-interval", cfg.discoveryRetentionInterval, "discovery retention schedule interval")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		return serve(cfg)
	case "db":
		if len(args) < 2 {
			usage()
			return fmt.Errorf("db command is required")
		}
		flags := flag.NewFlagSet("db "+args[1], flag.ContinueOnError)
		path := flags.String("path", cfg.databasePath, "SQLite database path")
		output := flags.String("output", "", "backup destination path (for db backup)")
		retentionDays := flags.Int("retention-days", 90, "report retention age in days (for db retention dry-run)")
		if err := flags.Parse(args[2:]); err != nil {
			return err
		}
		db, err := database.Open(*path)
		if err != nil {
			return err
		}
		defer db.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		switch args[1] {
		case "migrate":
			return database.Migrate(ctx, db)
		case "status":
			return database.MigrationStatus(ctx, db)
		case "verify":
			if err := database.Verify(ctx, db); err != nil {
				return err
			}
			fmt.Println("database integrity and foreign keys verified")
			return nil
		case "backup":
			if *output == "" {
				return fmt.Errorf("db backup requires -output path")
			}
			if err := database.Backup(ctx, db, *output); err != nil {
				return err
			}
			fmt.Printf("database snapshot written to %s\n", *output)
			return nil
		case "retention":
			if err := database.CheckSchema(ctx, db); err != nil {
				return err
			}
			if *retentionDays < 1 || *retentionDays > 3650 {
				return fmt.Errorf("retention-days must be between 1 and 3650")
			}
			cutoff := time.Now().UTC().AddDate(0, 0, -*retentionDays).Format(time.RFC3339)
			stats, err := database.RetentionDryRun(ctx, db, cutoff)
			if err != nil {
				return err
			}
			return json.NewEncoder(os.Stdout).Encode(stats)
		default:
			usage()
			return fmt.Errorf("unknown db command %q", args[1])
		}
	case "admin":
		if len(args) < 2 || (args[1] != "bootstrap" && args[1] != "password-reset") {
			usage()
			return fmt.Errorf("usage: portinaia admin {bootstrap|password-reset} -username name")
		}
		flags := flag.NewFlagSet("admin "+args[1], flag.ContinueOnError)
		path := flags.String("db", cfg.databasePath, "SQLite database path")
		username := flags.String("username", "", "initial administrator username")
		if err := flags.Parse(args[2:]); err != nil {
			return err
		}
		password := os.Getenv("PORTINAIA_ADMIN_PASSWORD")
		if password == "" {
			password = os.Getenv("PORTINAIA_BOOTSTRAP_PASSWORD")
		}
		if password == "" {
			return fmt.Errorf("PORTINAIA_ADMIN_PASSWORD must be set")
		}
		db, err := database.Open(*path)
		if err != nil {
			return err
		}
		defer db.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := database.CheckSchema(ctx, db); err != nil {
			return err
		}
		switch args[1] {
		case "bootstrap":
			return server.BootstrapAdministrator(db, *username, password)
		case "password-reset":
			return server.ResetAdministratorPassword(db, *username, password)
		default:
			return fmt.Errorf("unknown admin command %q", args[1])
		}
	case "version":
		fmt.Printf("portinaia %s\n", version.String())
		return nil
	default:
		usage()
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func serve(cfg config) error {
	if cfg.discoveryRetentionDays < 90 || cfg.discoveryRetentionDays > 3650 {
		return fmt.Errorf("discovery retention days must be between 90 and 3650 to preserve the retry window")
	}
	if cfg.discoveryRetentionBatch < 1 || cfg.discoveryRetentionBatch > 10000 {
		return fmt.Errorf("discovery retention batch must be between 1 and 10000")
	}
	if cfg.discoveryRetentionInterval < time.Minute || cfg.discoveryRetentionInterval > 24*time.Hour {
		return fmt.Errorf("discovery retention interval must be between 1m and 24h")
	}
	var proxyCIDRs []string
	for _, value := range strings.Split(cfg.trustedProxyCIDRs, ",") {
		if value = strings.TrimSpace(value); value != "" {
			proxyCIDRs = append(proxyCIDRs, value)
		}
	}
	handlerConfig := server.HandlerConfig{PublicURL: cfg.publicURL, TrustedProxyCIDRs: proxyCIDRs}
	if err := server.ValidateHandlerConfig(handlerConfig); err != nil {
		return err
	}
	db, err := database.Open(cfg.databasePath)
	if err != nil {
		return err
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	if err := database.CheckSchema(ctx, db); err != nil {
		cancel()
		return err
	}
	cancel()
	handler := server.NewConfiguredHandler(handlerConfig, db)
	srv := &http.Server{Addr: cfg.listenAddress, Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second}
	stop, stopCancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	retentionDone := make(chan struct{})
	shutdownDone := make(chan struct{})
	go func() {
		defer close(retentionDone)
		server.RunDiscoveryRetention(stop, db, time.Duration(cfg.discoveryRetentionDays)*24*time.Hour, cfg.discoveryRetentionBatch, cfg.discoveryRetentionInterval)
	}()
	go func() {
		defer close(shutdownDone)
		<-stop.Done()
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer shutdownCancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			log.Printf("http graceful shutdown failed: %v", err)
			_ = srv.Close()
		}
	}()
	log.Printf("portinaia listening on %s", cfg.listenAddress)
	serveErr := srv.ListenAndServe()
	stopCancel()
	<-shutdownDone
	<-retentionDone
	if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
		return serveErr
	}
	return nil
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: portinaia {server [-listen addr] [-db path] [-public-url url] [-trusted-proxy-cidrs cidrs] [-discovery-retention-days n] [-discovery-retention-batch n] [-discovery-retention-interval duration]|db {migrate|status|verify|retention} [-path path]|db backup -path dbfile -output backupfile|admin {bootstrap|password-reset} -username name|version}")
}
