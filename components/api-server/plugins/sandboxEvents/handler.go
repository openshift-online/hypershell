package sandboxEvents

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
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
			events := append([]Event(nil), snapshot.Events...)
			sort.SliceStable(events, func(i, j int) bool { return eventNewer(events[i], events[j]) })
			for _, event := range events {
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
	_ = json.NewEncoder(w).Encode(value)
}

func eventNewer(left, right Event) bool {
	leftTime, rightTime := eventTime(left), eventTime(right)
	if !leftTime.Equal(rightTime) {
		return leftTime.After(rightTime)
	}
	if left.InvolvedPodUID != right.InvolvedPodUID {
		return left.InvolvedPodUID < right.InvolvedPodUID
	}
	if left.InvolvedPodName != right.InvolvedPodName {
		return left.InvolvedPodName < right.InvolvedPodName
	}
	if left.Reason != right.Reason {
		return left.Reason < right.Reason
	}
	return left.Message < right.Message
}
func eventTime(event Event) time.Time {
	if event.LastObservedAt != nil {
		return *event.LastObservedAt
	}
	if event.FirstObservedAt != nil {
		return *event.FirstObservedAt
	}
	return time.Time{}
}
