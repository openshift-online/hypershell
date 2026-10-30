package sources

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestSdlcAgentsGrouping covers per-agent, per-env grouping: two agents across
// two envs must produce four rows in the payload, envs sorted alphabetically,
// agents sorted by (name, env).
// Verifies: SDLC-AGT-03.01
func TestSdlcAgentsGrouping(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		q := r.URL.Query().Get("query")
		switch {
		case r.URL.Path == "/api/v1/query_range":
			_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"matrix","result":[]}}`))
		case strings.Contains(q, "sum by (name, env) (sdlc_agent)"):
			_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[` +
				`{"metric":{"name":"reconciler","env":"prod"},"value":[1,"2"]},` +
				`{"metric":{"name":"reconciler","env":"staging"},"value":[1,"1"]},` +
				`{"metric":{"name":"spec-writer","env":"prod"},"value":[1,"3"]},` +
				`{"metric":{"name":"spec-writer","env":"staging"},"value":[1,"1"]}` +
				`]}}`))
		case strings.Contains(q, "count by (name, env) (sdlc_agent)"):
			_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[` +
				`{"metric":{"name":"reconciler","env":"prod"},"value":[1,"3"]},` +
				`{"metric":{"name":"reconciler","env":"staging"},"value":[1,"2"]},` +
				`{"metric":{"name":"spec-writer","env":"prod"},"value":[1,"3"]},` +
				`{"metric":{"name":"spec-writer","env":"staging"},"value":[1,"1"]}` +
				`]}}`))
		case strings.Contains(q, "sdlc_agent_queue_depth"):
			_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[` +
				`{"metric":{"name":"reconciler","env":"prod"},"value":[1,"5"]},` +
				`{"metric":{"name":"spec-writer","env":"prod"},"value":[1,"2"]}` +
				`]}}`))
		default:
			_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[]}}`))
		}
	}))
	defer srv.Close()

	p := &Prometheus{
		base:   srv.URL,
		client: srv.Client(),
		logger: slog.Default(),
	}

	out, err := p.SdlcAgents(context.Background())
	if err != nil {
		t.Fatalf("SdlcAgents: %v", err)
	}
	data, ok := out.(SdlcAgentsData)
	if !ok {
		t.Fatalf("SdlcAgents returned %T, want SdlcAgentsData", out)
	}

	// Envs sorted alphabetically.
	if len(data.Envs) != 2 || data.Envs[0] != "prod" || data.Envs[1] != "staging" {
		t.Errorf("Envs = %v, want [prod staging]", data.Envs)
	}

	// Four agent rows sorted by (name, env).
	if len(data.Agents) != 4 {
		t.Fatalf("got %d agents, want 4", len(data.Agents))
	}
	want := []struct {
		name string
		env  string
	}{
		{"reconciler", "prod"},
		{"reconciler", "staging"},
		{"spec-writer", "prod"},
		{"spec-writer", "staging"},
	}
	for i, w := range want {
		if data.Agents[i].Name != w.name || data.Agents[i].Env != w.env {
			t.Errorf("Agents[%d] = (%q, %q), want (%q, %q)", i, data.Agents[i].Name, data.Agents[i].Env, w.name, w.env)
		}
	}

	// Verify a specific scalar (queueDepth for reconciler/prod).
	if data.Agents[0].QueueDepth != 5 {
		t.Errorf("reconciler/prod QueueDepth = %v, want 5", data.Agents[0].QueueDepth)
	}
}

// TestSdlcAgentsReplicas covers replicasOn vs replicasTotal: sum(sdlc_agent) is
// replicasOn (running replicas) while count(sdlc_agent) is replicasTotal (all
// registered replicas including those reporting 0).
// Verifies: SDLC-AGT-03.02
func TestSdlcAgentsReplicas(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		q := r.URL.Query().Get("query")
		switch {
		case r.URL.Path == "/api/v1/query_range":
			_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"matrix","result":[]}}`))
		case strings.Contains(q, "sum by (name, env) (sdlc_agent)"):
			// 2 of 3 replicas running.
			_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[` +
				`{"metric":{"name":"builder","env":"prod"},"value":[1,"2"]}` +
				`]}}`))
		case strings.Contains(q, "count by (name, env) (sdlc_agent)"):
			// 3 total replicas registered.
			_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[` +
				`{"metric":{"name":"builder","env":"prod"},"value":[1,"3"]}` +
				`]}}`))
		default:
			_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[]}}`))
		}
	}))
	defer srv.Close()

	p := &Prometheus{
		base:   srv.URL,
		client: srv.Client(),
		logger: slog.Default(),
	}

	out, err := p.SdlcAgents(context.Background())
	if err != nil {
		t.Fatalf("SdlcAgents: %v", err)
	}
	data := out.(SdlcAgentsData)
	if len(data.Agents) != 1 {
		t.Fatalf("got %d agents, want 1", len(data.Agents))
	}
	a := data.Agents[0]
	if a.ReplicasOn != 2 {
		t.Errorf("ReplicasOn = %d, want 2", a.ReplicasOn)
	}
	if a.ReplicasTotal != 3 {
		t.Errorf("ReplicasTotal = %d, want 3", a.ReplicasTotal)
	}
}

// TestSdlcAgentsHistoryAlignment covers the per-agent shared time axis: all four
// series arrays (queueDepth, itemsProcessed, itemsFailed, provisionSeconds) must
// be index-aligned to the same sorted timestamp axis.
// Verifies: SDLC-AGT-04.02
func TestSdlcAgentsHistoryAlignment(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		q := r.URL.Query().Get("query")
		switch {
		case r.URL.Path == "/api/v1/query_range" && strings.Contains(q, "queue_depth"):
			_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"matrix","result":[` +
				`{"metric":{"name":"agent-a","env":"prod"},"values":[[100,"10"],[200,"20"]]}` +
				`]}}`))
		case r.URL.Path == "/api/v1/query_range" && strings.Contains(q, "items_processed"):
			_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"matrix","result":[` +
				`{"metric":{"name":"agent-a","env":"prod"},"values":[[100,"50"],[200,"60"]]}` +
				`]}}`))
		case r.URL.Path == "/api/v1/query_range" && strings.Contains(q, "items_failed"):
			// Only one sample at ts 200 - the ts 100 column must zero-fill.
			_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"matrix","result":[` +
				`{"metric":{"name":"agent-a","env":"prod"},"values":[[200,"3"]]}` +
				`]}}`))
		case r.URL.Path == "/api/v1/query_range" && strings.Contains(q, "provision_seconds"):
			_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"matrix","result":[` +
				`{"metric":{"name":"agent-a","env":"prod"},"values":[[100,"1.5"],[200,"2.5"]]}` +
				`]}}`))
		default:
			_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[]}}`))
		}
	}))
	defer srv.Close()

	p := &Prometheus{
		base:   srv.URL,
		client: srv.Client(),
		logger: slog.Default(),
	}

	out, err := p.SdlcAgents(context.Background())
	if err != nil {
		t.Fatalf("SdlcAgents: %v", err)
	}
	data := out.(SdlcAgentsData)
	if len(data.Agents) != 1 {
		t.Fatalf("got %d agents, want 1", len(data.Agents))
	}
	a := data.Agents[0]

	// Shared time axis.
	if got := a.Series.Times; len(got) != 2 || got[0] != 100 || got[1] != 200 {
		t.Fatalf("Series.Times = %v, want [100 200]", got)
	}

	// All four series are index-aligned to the two-column axis.
	if want := []float64{10, 20}; !floatsEqual(a.Series.QueueDepth, want) {
		t.Errorf("QueueDepth = %v, want %v", a.Series.QueueDepth, want)
	}
	if want := []float64{50, 60}; !floatsEqual(a.Series.ItemsProcessed, want) {
		t.Errorf("ItemsProcessed = %v, want %v", a.Series.ItemsProcessed, want)
	}
	// itemsFailed had no sample at ts 100, so the zero-filled column appears.
	if want := []float64{0, 3}; !floatsEqual(a.Series.ItemsFailed, want) {
		t.Errorf("ItemsFailed = %v, want %v", a.Series.ItemsFailed, want)
	}
	if want := []float64{1.5, 2.5}; !floatsEqual(a.Series.ProvisionSeconds, want) {
		t.Errorf("ProvisionSeconds = %v, want %v", a.Series.ProvisionSeconds, want)
	}
}

// TestSdlcAgentsLastReportedAt covers the freshness fact: the instant query
// max by (name, env) (sdlc_agent_last_report_timestamp_seconds) is mapped onto
// each row as LastReportedAt (unix seconds), and defaults to 0 when the series is
// absent for an agent.
// Verifies: SDLC-AGT-15.01, SDLC-AGT-15.02
func TestSdlcAgentsLastReportedAt(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		q := r.URL.Query().Get("query")
		switch {
		case r.URL.Path == "/api/v1/query_range":
			_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"matrix","result":[]}}`))
		case strings.Contains(q, "sum by (name, env) (sdlc_agent)"):
			// Two agents report on the on/off gauge; only one reports a freshness fact.
			_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[` +
				`{"metric":{"name":"triage","env":"integration"},"value":[1,"1"]},` +
				`{"metric":{"name":"spec-review","env":"integration"},"value":[1,"0"]}` +
				`]}}`))
		case strings.Contains(q, "sdlc_agent_last_report_timestamp_seconds"):
			// Only triage has pushed a freshness timestamp.
			_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[` +
				`{"metric":{"name":"triage","env":"integration"},"value":[1,"1700000000"]}` +
				`]}}`))
		default:
			_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[]}}`))
		}
	}))
	defer srv.Close()

	p := &Prometheus{
		base:   srv.URL,
		client: srv.Client(),
		logger: slog.Default(),
	}

	out, err := p.SdlcAgents(context.Background())
	if err != nil {
		t.Fatalf("SdlcAgents: %v", err)
	}
	data := out.(SdlcAgentsData)
	byName := map[string]SdlcAgentMetrics{}
	for _, a := range data.Agents {
		byName[a.Name] = a
	}

	// SDLC-AGT-15.01: the freshness fact maps to the pushed unix seconds.
	if got := byName["triage"].LastReportedAt; got != 1700000000 {
		t.Errorf("triage LastReportedAt = %d, want 1700000000", got)
	}
	// SDLC-AGT-15.02: an agent with no freshness series defaults to 0.
	if got := byName["spec-review"].LastReportedAt; got != 0 {
		t.Errorf("spec-review LastReportedAt = %d, want 0 (absent series)", got)
	}
}

// TestSdlcAgentsNaN covers the NaN guard: a Prometheus NaN sample must collapse
// to 0 so it does not break JSON marshaling.
func TestSdlcAgentsNaN(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		q := r.URL.Query().Get("query")
		switch {
		case r.URL.Path == "/api/v1/query_range":
			_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"matrix","result":[]}}`))
		case strings.Contains(q, "sum by (name, env) (sdlc_agent)"):
			_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[` +
				`{"metric":{"name":"nan-agent","env":"dev"},"value":[1,"NaN"]}` +
				`]}}`))
		case strings.Contains(q, "count by (name, env) (sdlc_agent)"):
			_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[` +
				`{"metric":{"name":"nan-agent","env":"dev"},"value":[1,"+Inf"]}` +
				`]}}`))
		default:
			_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[]}}`))
		}
	}))
	defer srv.Close()

	p := &Prometheus{
		base:   srv.URL,
		client: srv.Client(),
		logger: slog.Default(),
	}

	out, err := p.SdlcAgents(context.Background())
	if err != nil {
		t.Fatalf("SdlcAgents: %v", err)
	}
	data := out.(SdlcAgentsData)
	if len(data.Agents) != 1 {
		t.Fatalf("got %d agents, want 1", len(data.Agents))
	}
	a := data.Agents[0]
	if a.ReplicasOn != 0 {
		t.Errorf("ReplicasOn = %d, want 0 (NaN collapsed)", a.ReplicasOn)
	}
	if a.ReplicasTotal != 0 {
		t.Errorf("ReplicasTotal = %d, want 0 (+Inf collapsed)", a.ReplicasTotal)
	}
}
