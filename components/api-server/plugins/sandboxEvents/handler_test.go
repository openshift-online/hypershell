package sandboxEvents

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/mux"
)

func TestHandlerGroupsEventsAndPreservesFullRepresentation(t *testing.T) {
	store := NewMemoryStore(time.Minute, 10, 10)
	defer store.Close()
	first := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	last := first.Add(time.Minute)
	store.Replace("gateway-ns", "writer", []Event{
		{Type: "Warning", Reason: "recent", Message: "recent message", FirstObservedAt: &first, LastObservedAt: &last, Count: 2, InvolvedPodName: "writer-pod", InvolvedPodUID: "writer-uid"},
		{Type: "Warning", Reason: "old", Message: "old message", FirstObservedAt: &first, LastObservedAt: &first, Count: 1, InvolvedPodName: "writer-pod", InvolvedPodUID: "writer-uid"},
	})
	store.Replace("gateway-ns", "reviewer", []Event{{Type: "Normal", Reason: "Started", Message: "sandbox started", Count: 1, InvolvedPodName: "reviewer-pod", InvolvedPodUID: "reviewer-uid"}})

	h := NewHandler(store, func(_ context.Context, gatewayID string) (string, bool) {
		return "gateway-ns", gatewayID == "gateway-id"
	})
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request = mux.SetURLVars(request, map[string]string{"gateway_id": "gateway-id"})
	response := httptest.NewRecorder()
	h.List(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", response.Code, response.Body.String())
	}
	var got struct {
		Items []struct {
			SandboxName string `json:"sandbox_name"`
			Events      []struct {
				Type            string     `json:"type"`
				Reason          string     `json:"reason"`
				Message         string     `json:"message"`
				FirstObservedAt *time.Time `json:"first_observed_at"`
				LastObservedAt  *time.Time `json:"last_observed_at"`
				Count           int32      `json:"count"`
				InvolvedPodName string     `json:"involved_pod_name"`
				InvolvedPodUID  string     `json:"involved_pod_uid"`
			} `json:"events"`
		} `json:"items"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(got.Items) != 2 || got.Items[0].SandboxName != "reviewer" || got.Items[1].SandboxName != "writer" {
		t.Fatalf("groups = %#v, want reviewer and writer", got.Items)
	}
	writer := got.Items[1].Events
	if len(writer) != 2 || writer[0].Reason != "recent" || writer[1].Reason != "old" {
		t.Fatalf("writer event order = %#v", writer)
	}
	recent := writer[0]
	if recent.Type != "Warning" || recent.Reason != "recent" || recent.Message != "recent message" || recent.FirstObservedAt == nil || !recent.FirstObservedAt.Equal(first) || recent.LastObservedAt == nil || !recent.LastObservedAt.Equal(last) || recent.Count != 2 || recent.InvolvedPodName != "writer-pod" || recent.InvolvedPodUID != "writer-uid" {
		t.Fatalf("event representation = %#v", recent)
	}
}

func TestHandlerPreservesGSE01ReportedOrder(t *testing.T) {
	store := NewMemoryStore(time.Minute, 10, 10)
	defer store.Close()

	first := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	// The control plane received newer-event-time after older-event-time, so it
	// reported this GSE-01 order to the store. EventTime is not in the REST
	// representation. Sorting these fields would incorrectly put the older event
	// first because its series last-observed time is later.
	newerSeriesLastObserved := first.Add(time.Minute)
	olderSeriesLastObserved := first.Add(2 * time.Minute)
	store.Replace("gateway-ns", "writer", []Event{
		{Reason: "newer-event-time", FirstObservedAt: &first, LastObservedAt: &newerSeriesLastObserved},
		{Reason: "older-event-time", FirstObservedAt: &first, LastObservedAt: &olderSeriesLastObserved},
	})

	h := NewHandler(store, func(_ context.Context, gatewayID string) (string, bool) {
		return "gateway-ns", gatewayID == "gateway-id"
	})
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request = mux.SetURLVars(request, map[string]string{"gateway_id": "gateway-id"})
	response := httptest.NewRecorder()
	h.List(response, request)

	var got struct {
		Items []struct {
			Events []struct {
				Reason string `json:"reason"`
			} `json:"events"`
		} `json:"items"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(got.Items) != 1 || len(got.Items[0].Events) != 2 || got.Items[0].Events[0].Reason != "newer-event-time" || got.Items[0].Events[1].Reason != "older-event-time" {
		t.Fatalf("reported event order = %#v, want newer-event-time then older-event-time", got.Items)
	}
}

func TestHandlerReturnsEmptyCollection(t *testing.T) {
	store := NewMemoryStore(time.Minute, 10, 10)
	defer store.Close()
	h := NewHandler(store)
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request = mux.SetURLVars(request, map[string]string{"gateway_id": "none"})
	response := httptest.NewRecorder()
	h.List(response, request)
	if response.Code != http.StatusOK || response.Body.String() != "{\"items\":[]}\n" {
		t.Fatalf("empty response: %d %s", response.Code, response.Body.String())
	}
}
