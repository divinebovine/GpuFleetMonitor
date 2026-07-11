package main

import (
	"context"
	"testing"

	"github.com/divinebovine/GpuFleetMonitor/internal/gpu"
)

func TestRunWorkerPool(t *testing.T) {
	ids := gpu.AllIDs()[:20]
	results := runWorkerPool(context.Background(), ids)

	seen := make(map[string]bool)
	for h := range results {
		if seen[h.GPUID] {
			t.Errorf("duplicate result for %s", h.GPUID)
		}
		seen[h.GPUID] = true
	}

	if len(seen) != len(ids) {
		t.Errorf("expected %d results, got %d", len(ids), len(seen))
	}
}

func TestRunWorkerPoolCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	results := runWorkerPool(ctx, gpu.AllIDs())

	var count int
	for range results {
		count++
	}

	// We can't assert count == 0: the jobs channel is buffered to workerPoolSize,
	// and Go's select picks randomly when multiple cases are ready, so up to
	// workerPoolSize jobs can be enqueued before ctx.Done() wins. The reliable
	// invariant is that we didn't process all GPUs.
	if count == int(gpu.TotalGpus) {
		t.Errorf("expected cancellation to reduce results, got all %d", gpu.TotalGpus)
	}
}
