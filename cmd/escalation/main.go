package main

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/divinebovine/GpuFleetMonitor/internal/escalation"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func main() {
	h := &handler{store: escalation.NewStore()}
	r := chi.NewRouter()

	// middleware
	r.Use(middleware.RequestID)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer) // Recovers from panics

	// setup a timeout for the context
	r.Use(middleware.Timeout(60 * time.Second))

	r.Route("/v1", func(r chi.Router) {
		r.Post("/escalations/{id}", h.createEscalationHandler)
		r.Get("/escalations/{id}", h.getEscalationByIDHandler)
		r.Get("/escalations", h.getEscalationsHandler)
		r.Put("/escalations/{id}/resolve", h.resolveEscalationHandler)
	})

	log.Fatal(http.ListenAndServe(":8082", r))
}

type handler struct {
	store *escalation.Store
}

func (h *handler) createEscalationHandler(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	_, exists := h.store.GetByID(id)

	if exists {
		http.Error(w, "Bad Request: Escalation ID already exists", http.StatusBadRequest)
		return
	}

	var e *escalation.Escalation
	if err := json.NewDecoder(r.Body).Decode(&e); err != nil {
		http.Error(w, "Bad Request: Unable to process payload", http.StatusBadRequest)
		return
	}

	if e == nil {
		http.Error(w, "Bad Request: Unable to process payload", http.StatusBadRequest)
		return
	}

	h.store.Save(*e)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(e)
}

func (h *handler) getEscalationByIDHandler(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	e, ok := h.store.GetByID(id)

	if !ok {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(e)
}

func (h *handler) getEscalationsHandler(w http.ResponseWriter, r *http.Request) {
	all := h.store.List()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(all)
}

func (h *handler) resolveEscalationHandler(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	e, ok := h.store.GetByID(id)

	if !ok {
		http.NotFound(w, r)
		return
	}

	e.Resolve(time.Now().UTC())
	h.store.Save(e)
	w.WriteHeader(http.StatusNoContent)
}
