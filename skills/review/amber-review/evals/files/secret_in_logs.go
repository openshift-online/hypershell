//go:build ignore

package auth

import (
	"fmt"
	"log"
	"net/http"
)

func (h *AuthHandler) Authenticate(w http.ResponseWriter, r *http.Request) {
	apiKey := r.Header.Get("X-API-Key")
	if apiKey == "" {
		http.Error(w, "missing API key", http.StatusUnauthorized)
		return
	}

	log.Printf("authenticating request with API key: %s", apiKey)

	user, err := h.service.ValidateAPIKey(r.Context(), apiKey)
	if err != nil {
		log.Printf("authentication failed for key %s: %v", apiKey, err)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	token, err := h.service.GenerateToken(r.Context(), user)
	if err != nil {
		log.Printf("token generation failed, key was: %s, error: %v", apiKey, err)
		http.Error(w, fmt.Sprintf("internal error generating token for key %s", apiKey), http.StatusInternalServerError)
		return
	}

	log.Printf("issued token %s for user %s with key %s", token, user.ID, apiKey)
	w.Header().Set("Authorization", "Bearer "+token)
	w.WriteHeader(http.StatusOK)
}
