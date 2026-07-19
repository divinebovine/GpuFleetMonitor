package main

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
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

func TestGetAllGPUsHandlerJSON(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/v1/gpus", nil)
	w := httptest.NewRecorder()

	getAllGPUsHandler(w, req)

	res := w.Result()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", res.StatusCode)
	}
	if ct := res.Header.Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Errorf("expected application/json content-type, got %q", ct)
	}

	var healths []gpu.GPUHealth
	if err := json.NewDecoder(res.Body).Decode(&healths); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(healths) != int(gpu.TotalGpus) {
		t.Errorf("expected %d GPUs, got %d", gpu.TotalGpus, len(healths))
	}
}

func TestGetAllGPUsHandlerSSE(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(getAllGPUsHandler))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/v1/gpus", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Accept", "text/event-stream")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("expected text/event-stream content-type, got %q", ct)
	}

	var dataCount int
	gotDone := false
	namedEvent := false
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "event:") {
			namedEvent = true
			if line == "event: done" {
				gotDone = true
				cancel() // disconnect: stops the live-update loop on the server side
				break
			}
		} else if strings.HasPrefix(line, "data:") && !namedEvent {
			dataCount++
		} else if line == "" {
			namedEvent = false
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scanner error: %v", err)
	}

	if dataCount != int(gpu.TotalGpus) {
		t.Errorf("expected %d data events, got %d", gpu.TotalGpus, dataCount)
	}
	if !gotDone {
		t.Error("expected event: done, not received")
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
