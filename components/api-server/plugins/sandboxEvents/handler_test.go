package sandboxEvents

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/mux"
)

func TestHandlerGroupsOrdersAndReturnsEmpty(t *testing.T) {
	store := NewMemoryStore(time.Minute, 10, 10)
	defer store.Close()
	old, recent := time.Now().Add(-time.Minute), time.Now()
	store.Replace("gw", "writer", []Event{{Reason: "old", LastObservedAt: &old}, {Reason: "recent", LastObservedAt: &recent}})
	h := NewHandler(store)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req = mux.SetURLVars(req, map[string]string{"gateway_id": "gw"})
	response := httptest.NewRecorder()
	h.List(response, req)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"reason":"recent"`) || strings.Index(response.Body.String(), `"reason":"recent"`) > strings.Index(response.Body.String(), `"reason":"old"`) {
		t.Fatalf("unexpected response: %d %s", response.Code, response.Body.String())
	}
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req = mux.SetURLVars(req, map[string]string{"gateway_id": "none"})
	response = httptest.NewRecorder()
	h.List(response, req)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"items":[]`) {
		t.Fatalf("empty response: %d %s", response.Code, response.Body.String())
	}
}
