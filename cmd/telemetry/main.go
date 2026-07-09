package main

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
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
	})

	log.Fatal(http.ListenAndServe(":3000", r))
}

func getGpuHandler(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	gpuHealth, err := gpu.GetHealth(id)

	if err != nil {
		http.NotFound(w, r) // just use not found for now
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(gpuHealth)
}

func getAllGPUsHandler(w http.ResponseWriter, r *http.Request) {
	var healths []*gpu.GPUHealth
	allIDs := gpu.AllIDs()

	jobs := make(chan string, workerPoolSize)
	results := make(chan *gpu.GPUHealth, len(allIDs))
	var wg sync.WaitGroup

	go func() {
		for _, id := range allIDs {
			jobs <- id
		}
		close(jobs)
	}()

	for range workerPoolSize {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for id := range jobs {
				gpuHealth, err := gpu.GetHealth(id)

				if err != nil {
					// do something
					return
				}

				results <- gpuHealth
			}
		}()
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	for h := range results {
		healths = append(healths, h)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(healths)
}
