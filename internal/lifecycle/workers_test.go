package lifecycle

import (
	"context"
	"errors"
	"testing"
)

func TestRunJoinsWorkersAndPreservesCleanupErrors(t *testing.T) {
	failed := errors.New("worker failed")
	cleanup := errors.New("cleanup failed")
	joined := make(chan struct{})
	err := Run(context.Background(),
		func(context.Context) error { return failed },
		func(ctx context.Context) error {
			<-ctx.Done()
			defer close(joined)
			return cleanup
		},
	)
	if !errors.Is(err, failed) || !errors.Is(err, cleanup) {
		t.Fatalf("lost worker errors: %v", err)
	}
	select {
	case <-joined:
	default:
		t.Fatal("returned before worker cleanup completed")
	}
}

func TestRunCancellationIsNormalStop(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := Run(ctx, func(ctx context.Context) error { return ctx.Err() }); err != nil {
		t.Fatal(err)
	}
}
