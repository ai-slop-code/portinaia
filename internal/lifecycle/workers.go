// Package lifecycle supplies the join-on-exit worker lifecycle shared by services.
package lifecycle

import (
	"context"
	"errors"
)

type Worker func(context.Context) error

// Run cancels siblings when any worker exits and joins every worker before
// returning. Cancellation is a normal stop; other errors from any worker survive.
// Workers must honor cancellation, including bounded cleanup where needed.
func Run(ctx context.Context, workers ...Worker) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	results := make(chan error, len(workers))
	for _, worker := range workers {
		go func() { results <- worker(ctx) }()
	}
	var result error
	for range workers {
		err := <-results
		if err != nil && !(ctx.Err() != nil && errors.Is(err, context.Canceled)) {
			result = errors.Join(result, err)
		}
		cancel()
	}
	return result
}
