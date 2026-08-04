package handlers

import (
	"net/http"

	"github.com/gorilla/mux"
)

func (h *GatewayHandler) DeleteGateway(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	gateway, err := h.service.Get(r.Context(), id)
	if err != nil {
		panic("failed to get gateway: " + err.Error())
	}

	if gateway.Status == "deleting" {
		panic("gateway is already being deleted")
	}

	err = h.service.Delete(r.Context(), id)
	if err != nil {
		panic("failed to delete gateway")
	}

	w.WriteHeader(http.StatusNoContent)
}
