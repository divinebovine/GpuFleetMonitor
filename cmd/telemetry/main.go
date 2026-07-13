package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/divinebovine/GpuFleetMonitor/internal/gpu"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

const workerPoolSize = 100

func main() {
	r := chi.NewRouter()

	// middleware
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP) // Pick the correct middleware for your setup
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer) // Recovers from panics

	// Set a timeout for the context
	r.Use(middleware.Timeout(60 * time.Second))

	r.Route("/v1", func(r chi.Router) {
		r.Get("/gpus/{id}", getGpuHandler)
		r.Get("/gpus", getAllGPUsHandler)
		r.Get("/simulation/settings", getSimulationSettingsHandler)
		r.Put("/simulation/settings", putSimulationSettingsHandler)
		r.Post("/simulation/settings/reset", postResetSimulationSettingsHandler)
	})

	// Start simulation ticker
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	gpu.DefaultTicker.Start(ctx)

	log.Fatal(http.ListenAndServe(":3000", r))
}

func getGpuHandler(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	gpuHealth, err := gpu.GetHealth(r.Context(), id)

	if err != nil {
		http.NotFound(w, r) // just use not found for now
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(gpuHealth)
}

func getAllGPUsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("Vary", "Accept")
	accept := r.Header.Get("Accept")
	switch {
	case strings.Contains(accept, "text/event-stream"):
		eventStreamAllGPUs(w, r)
	default:
		fetchAllGPUs(w, r)
	}
}

func eventStreamAllGPUs(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "event streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")

	ctx := r.Context()
	results := runWorkerPool(ctx, gpu.AllIDs())
	for {
		select {
		case h, ok := <-results:
			if !ok {
				fmt.Fprintf(w, "event: done\ndata: {}\n\n")
				flusher.Flush()
				return
			}

			data, err := json.Marshal(h)
			if err != nil {
				log.Printf("failed to marshal GPU health for %s: %v", h.GPUID, err)
				continue
			}

			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		case <-ctx.Done():
			return
		}
	}
}

func fetchAllGPUs(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	results := runWorkerPool(ctx, gpu.AllIDs())
	var healths []*gpu.GPUHealth

	for {
		select {
		case h, ok := <-results:
			if !ok {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(healths)
				return
			}
			healths = append(healths, h)
		case <-ctx.Done():
			return
		}
	}
}

func runWorkerPool(ctx context.Context, allIDs []string) <-chan *gpu.GPUHealth {
	jobs := make(chan string, workerPoolSize)
	results := make(chan *gpu.GPUHealth, workerPoolSize)
	var wg sync.WaitGroup

	go func() {
		for _, id := range allIDs {
			select {
			case jobs <- id:
			case <-ctx.Done():
				close(jobs)
				return
			}
		}
		close(jobs)
	}()

	for range workerPoolSize {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for id := range jobs {
				h, err := gpu.GetHealth(ctx, id)

				if err != nil {
					// probably should have some way to alert that
					// a gpu is not responding to health checks
					continue
				}

				select {
				case results <- h:
				case <-ctx.Done():
					return
				}
			}
		}()
	}

	go func() {
		wg.Wait()
		close(results)
	}()
	return results
}

func getSimulationSettingsHandler(w http.ResponseWriter, r *http.Request) {
	settings := gpu.Config.Get()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(settings)
}

func putSimulationSettingsHandler(w http.ResponseWriter, r *http.Request) {
	var settings *gpu.SimulationSettings = &gpu.SimulationSettings{}
	err := json.NewDecoder(r.Body).Decode(settings)

	if err != nil {
		http.Error(w, "unable to parse settings", http.StatusBadRequest)
		return
	}

	if settings.SpeedMultiplier < 1 || settings.SpeedMultiplier > 100 {
		http.Error(w, "speed multiplier must be between 1 and 100", http.StatusBadRequest)
		return
	}

	if settings.HealthyToWarningRate < 0 || settings.HealthyToWarningRate > 1.0 {
		http.Error(w, "healthy to warning rate must be greater than or equal to 0.0 and less than or equal to 1.0", http.StatusBadRequest)
		return
	}

	if settings.WarningToCriticalRate < 0 || settings.WarningToHealthyRate < 0 {
		http.Error(w, "warning to critical rate and warning to healthy rate cannot be less than 0.0", http.StatusBadRequest)
		return
	}

	if settings.WarningToCriticalRate+settings.WarningToHealthyRate >= 1.0 {
		http.Error(w, "warning to critical rate and warning to healthy rate cannot sum to a number greater than or equal to 1.0", http.StatusBadRequest)
		return
	}

	confirmedSettings := gpu.Config.Set(settings)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(confirmedSettings)
}

func postResetSimulationSettingsHandler(w http.ResponseWriter, r *http.Request) {
	confirmedSettings := gpu.Config.Reset()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(confirmedSettings)
}
