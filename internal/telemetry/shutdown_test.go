package telemetry

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestShutdownOffersEverySignalTheBudgetAndPreservesErrors(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	started := make(chan struct{})
	otherFailure := errors.New("other exporter failure")
	var calls atomic.Int32
	rt := &Runtime{shutdown: []func(context.Context) error{
		func(ctx context.Context) error {
			calls.Add(1)
			select {
			case <-started:
				return context.DeadlineExceeded
			case <-ctx.Done():
				t.Error("one signal starved another signal of its shutdown budget")
				return ctx.Err()
			}
		},
		func(context.Context) error {
			calls.Add(1)
			close(started)
			return otherFailure
		},
	}}
	for range 2 {
		err := rt.Shutdown(ctx)
		if !errors.Is(err, context.DeadlineExceeded) || !errors.Is(err, otherFailure) {
			t.Fatalf("lost exporter shutdown failures: %v", err)
		}
	}
	if calls.Load() != 2 {
		t.Fatalf("shutdown called providers %d times", calls.Load())
	}
}
