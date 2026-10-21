package sandboxEvents

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/mux"
)

// NamespaceResolver converts a public gateway id to the gateway namespace used
// as the cache tenancy key. Authorization runs before this resolver.
type NamespaceResolver func(context.Context, string) (string, bool)

type handler struct {
	store            SandboxEventStore
	resolveNamespace NamespaceResolver
}

func NewHandler(store SandboxEventStore, resolvers ...NamespaceResolver) *handler {
	resolver := NamespaceResolver(func(_ context.Context, value string) (string, bool) {
		return value, value != ""
	})
	if len(resolvers) > 0 && resolvers[0] != nil {
		resolver = resolvers[0]
	}
	return &handler{store: store, resolveNamespace: resolver}
}

type response struct {
	Items []groupResponse `json:"items"`
}
type groupResponse struct {
	SandboxName string          `json:"sandbox_name"`
	ObservedAt  time.Time       `json:"observed_at"`
	Events      []eventResponse `json:"events"`
}
type eventResponse struct {
	Type            string     `json:"type,omitempty"`
	Reason          string     `json:"reason,omitempty"`
	Message         string     `json:"message,omitempty"`
	FirstObservedAt *time.Time `json:"first_observed_at,omitempty"`
	LastObservedAt  *time.Time `json:"last_observed_at,omitempty"`
	Count           int32      `json:"count"`
	InvolvedPodName string     `json:"involved_pod_name,omitempty"`
	InvolvedPodUID  string     `json:"involved_pod_uid,omitempty"`
}

func (h *handler) List(w http.ResponseWriter, r *http.Request) {
	out := response{Items: make([]groupResponse, 0)}
	if h.store != nil {
		namespace, found := h.resolveNamespace(r.Context(), mux.Vars(r)["gateway_id"])
		if !found {
			writeSandboxEventsJSON(w, out)
			return
		}
		for _, snapshot := range h.store.List(namespace) {
			group := groupResponse{SandboxName: snapshot.Sandbox, ObservedAt: snapshot.ObservedAt, Events: make([]eventResponse, 0, len(snapshot.Events))}
			// The control plane establishes OSG-SEVT-01 order before it projects source
			// Kubernetes events into this REST representation. Preserve that reported
			// order because EventTime and creation timestamp are not available here.
			for _, event := range snapshot.Events {
				group.Events = append(group.Events, eventResponse(event))
			}
			out.Items = append(out.Items, group)
		}
	}
	writeSandboxEventsJSON(w, out)
}

func writeSandboxEventsJSON(w http.ResponseWriter, value response) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Printf("WARN sandbox events: encode response: %v", err)
	}
}
