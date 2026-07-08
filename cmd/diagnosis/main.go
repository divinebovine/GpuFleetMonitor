package main

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/divinebovine/gpu-monitor/internal/diagnosis"
	"github.com/divinebovine/gpu-monitor/internal/gpu"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func main() {
	h := &handler{store: diagnosis.NewStore()}
	r := chi.NewRouter()

	// middleware
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP) // Pick the correct middleware for your setup
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer) // Recovers from panics

	// Set a timeout for the context
	r.Use(middleware.Timeout((60 * time.Second)))

	r.Route("/v1", func(r chi.Router) {
		r.Post("/diagnose/{gpu_id}", h.postDiagnosis)
		r.Get("/diagnose/{id}", h.getDiagnosis)
		r.Get("/diagnoses", h.getDiagnoses)
	})

	http.ListenAndServe(":8081", r)
}

type handler struct {
	store *diagnosis.Store
}

func (h *handler) postDiagnosis(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "gpu_id")
	health, err := gpu.GetHealth(id)

	if err != nil {
		http.NotFound(w, r) // just use not found for now
		return
	}

	d := diagnosis.Analyze(health, time.Now().UTC())
	h.store.Save(d)

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(d)
}

func (h *handler) getDiagnosis(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	d, ok := h.store.GetByID(id)

	if !ok {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(d)
}

func (h *handler) getDiagnoses(w http.ResponseWriter, r *http.Request) {
	d := h.store.List()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(d)
}
