//go:build ignore

package handlers

import (
	"errors"
	"fmt"
	"log"
	"net/http"

	"github.com/gorilla/mux"
)

func (h *FleetHandler) GetFleet(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	fleet, err := h.service.Get(r.Context(), id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			http.Error(w, fmt.Sprintf("fleet %s not found", id), http.StatusNotFound)
			return
		}
		log.Printf("failed to get fleet id=%s: %v", id, err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	if err := writeJSON(w, http.StatusOK, fleet); err != nil {
		log.Printf("failed to write fleet response: %v", err)
	}
}
