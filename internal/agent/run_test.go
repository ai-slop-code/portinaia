package agent

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/url"
	"os"
	"testing"
	"time"
)

func TestRetryPolicy(t *testing.T) {
	for _, test := range []struct {
		name  string
		err   error
		retry bool
		floor time.Duration
	}{
		{"local failure", os.ErrPermission, false, 0},
		{"revoked", &HTTPError{StatusCode: 401, Permanent: true}, false, 0},
		{"network", &url.Error{Op: "Post", Err: io.EOF}, true, 0},
		{"server backoff", &HTTPError{StatusCode: 429, RetryAfter: time.Hour}, true, time.Hour},
	} {
		t.Run(test.name, func(t *testing.T) {
			delay, retry := retryPolicy(test.err, 100)
			if retry != test.retry || delay < test.floor || delay > time.Hour {
				t.Fatalf("retry=%v delay=%s", retry, delay)
			}
		})
	}
}

func TestPeriodicLocalFailureIsTerminalAndCancellationIsClean(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	localFailure := errors.New("invalid local state")
	if err := runPeriodic(context.Background(), logger, "heartbeat", time.Hour, func(context.Context) error { return localFailure }); !errors.Is(err, localFailure) {
		t.Fatalf("local failure was swallowed: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := runPeriodic(ctx, logger, "snapshot", time.Hour, func(ctx context.Context) error {
		cancel()
		return ctx.Err()
	}); err != nil {
		t.Fatalf("cancellation was treated as terminal failure: %v", err)
	}
}

func TestPeriodicBackoffHonorsRetryAfterAndIsCancellable(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	calls := 0
	err := runPeriodic(ctx, slog.New(slog.NewTextHandler(io.Discard, nil)), "snapshot", time.Millisecond, func(context.Context) error {
		calls++
		return &HTTPError{StatusCode: 429, RetryAfter: time.Hour}
	})
	if err != nil || calls != 1 {
		t.Fatalf("backoff bypassed or cancellation failed: calls=%d error=%v", calls, err)
	}
}
