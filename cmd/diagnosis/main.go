package main

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/divinebovine/GpuFleetMonitor/internal/diagnosis"
	"github.com/divinebovine/GpuFleetMonitor/internal/gpu"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func main() {
	h := &handler{store: diagnosis.NewStore()}
	r := chi.NewRouter()

	// middleware
	r.Use(middleware.RequestID)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer) // Recovers from panics

	// Set a timeout for the context
	r.Use(middleware.Timeout((60 * time.Second)))

	r.Route("/v1", func(r chi.Router) {
		r.Post("/diagnose/{gpu_id}", h.postDiagnosis)
		r.Get("/diagnose/{id}", h.getDiagnosis)
		r.Get("/diagnoses", h.getDiagnoses)
	})

	log.Fatal(http.ListenAndServe(":8081", r))
}

type handler struct {
	store *diagnosis.Store
}

func (h *handler) postDiagnosis(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "gpu_id")
	health, err := gpu.GetHealth(r.Context(), id)

	if err != nil {
		http.NotFound(w, r) // just use not found for now
		return
	}

	d := diagnosis.Analyze(health, time.Now().UTC())
	h.store.Save(d)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(d)
}

func (h *handler) getDiagnosis(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	d, ok := h.store.GetByID(id)

	if !ok {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(d)
}

func (h *handler) getDiagnoses(w http.ResponseWriter, r *http.Request) {
	d := h.store.List()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(d)
}
