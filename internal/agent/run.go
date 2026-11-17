package agent

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"time"

	"portinaia/internal/lifecycle"
)

type RunConfig struct {
	StatePath         string
	HeartbeatInterval time.Duration
	SnapshotInterval  time.Duration
	PodmanEndpoints   []PodmanEndpoint
	Logger            *slog.Logger
}

// Run executes independent heartbeat and snapshot workers. Intervals are local
// operator overrides; server schedules never silently rewrite local policy.
func Run(ctx context.Context, config RunConfig) error {
	if config.HeartbeatInterval < 15*time.Second || config.HeartbeatInterval > 10*time.Minute {
		return errors.New("heartbeat interval must be between 15s and 10m")
	}
	if config.SnapshotInterval < time.Minute || config.SnapshotInterval > 24*time.Hour {
		return errors.New("snapshot interval must be between 1m and 24h")
	}
	if err := ValidatePodmanEndpoints(config.PodmanEndpoints); err != nil {
		return err
	}
	logger := config.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return lifecycle.Run(ctx,
		func(ctx context.Context) error {
			return runPeriodic(ctx, logger, "heartbeat", config.HeartbeatInterval, func(ctx context.Context) error {
				return HeartbeatContext(ctx, config.StatePath)
			})
		},
		func(ctx context.Context) error {
			return runPeriodic(ctx, logger, "snapshot", config.SnapshotInterval, func(ctx context.Context) error {
				return SnapshotContext(ctx, config.StatePath, config.PodmanEndpoints...)
			})
		},
	)
}

func runPeriodic(ctx context.Context, logger *slog.Logger, name string, interval time.Duration, operation func(context.Context) error) error {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for attempt := 0; ; {
		if ctx.Err() != nil {
			return nil
		}
		err := operation(ctx)
		if ctx.Err() != nil {
			return nil
		}
		if err == nil {
			attempt = 0
			logger.DebugContext(ctx, "agent operation completed", "operation", name)
			select {
			case <-ctx.Done():
				return nil
			case <-ticker.C:
			}
			continue
		}
		delay, retry := retryPolicy(err, attempt)
		if !retry {
			return fmt.Errorf("%s failed: %w", name, err)
		}
		// Error text from remote servers can contain arbitrary payloads. Export
		// structured status/correlation rather than remote details or credential-bearing URLs.
		attrs := []any{"operation", name, "retry_delay_seconds", delay.Seconds()}
		var responseError *HTTPError
		if errors.As(err, &responseError) {
			attrs = append(attrs, "http.response.status_code", responseError.StatusCode)
			if responseError.RequestID != "" {
				attrs = append(attrs, "request_id", responseError.RequestID)
			}
		}
		logger.WarnContext(ctx, "agent operation failed; retrying", attrs...)
		attempt++
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil
		case <-timer.C:
		}
	}
}

func retryPolicy(err error, attempt int) (time.Duration, bool) {
	var responseError *HTTPError
	if errors.As(err, &responseError) {
		if responseError.Permanent {
			return 0, false
		}
		delay := retryDelay(attempt)
		if responseError.RetryAfter > delay {
			delay = responseError.RetryAfter
		}
		return delay, true
	}
	var networkError *url.Error
	if errors.As(err, &networkError) {
		return retryDelay(attempt), true
	}
	return 0, false
}

func retryDelay(attempt int) time.Duration {
	if attempt > 8 {
		attempt = 8
	}
	if attempt < 0 {
		attempt = 0
	}
	ceiling := time.Second * time.Duration(1<<attempt)
	var random [8]byte
	if _, err := rand.Read(random[:]); err != nil {
		return ceiling / 2
	}
	return time.Duration(binary.LittleEndian.Uint64(random[:]) % uint64(ceiling+1))
}
