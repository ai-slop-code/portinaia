//go:build !linux

package agent

import (
	"context"
	"errors"
)

func CollectSnapshot(_ ...PodmanEndpoint) ([]byte, error) {
	return nil, errors.New("discovery collection is supported on Linux only")
}

func CollectSnapshotContext(ctx context.Context, _ ...PodmanEndpoint) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return CollectSnapshot()
}
