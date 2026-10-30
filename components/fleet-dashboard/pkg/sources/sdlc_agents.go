package sources

import (
	"context"
	"sort"
	"time"
)

// SdlcAgentSeries is one agent's history on a shared axis.
type SdlcAgentSeries struct {
	Times            []int64   `json:"times"`
	QueueDepth       []float64 `json:"queueDepth"`
	ItemsProcessed   []float64 `json:"itemsProcessed"`
	ItemsFailed      []float64 `json:"itemsFailed"`
	ProvisionSeconds []float64 `json:"provisionSeconds"`
}

// SdlcAgentMetrics is one (agent, env) row.
type SdlcAgentMetrics struct {
	Name             string  `json:"name"`
	Env              string  `json:"env"`
	ReplicasOn       int     `json:"replicasOn"`
	ReplicasTotal    int     `json:"replicasTotal"`
	QueueDepth       float64 `json:"queueDepth"`
	ItemsProcessed   float64 `json:"itemsProcessed"`
	ItemsFailed      float64 `json:"itemsFailed"`
	ProvisionSeconds float64 `json:"provisionSeconds"`
	// LastReportedAt is the unix seconds of the agent's most recent freshness
	// push (SDLC-AGT-15), the absolute fact the client judges liveness against.
	// 0 when the freshness series is absent. The BFF reports only this fact; it
	// does NOT compute any staleness or activity verdict (a server verdict would
	// go stale inside the cached snapshot).
	LastReportedAt int64           `json:"lastReportedAt"`
	Series         SdlcAgentSeries `json:"series"`
}

// SdlcAgentsData is the /api/sdlc-agents plane payload (the Snapshot.Data value).
type SdlcAgentsData struct {
	Envs   []string           `json:"envs"`
	Agents []SdlcAgentMetrics `json:"agents"`
}

// agentKey groups metrics by (name, env) pair.
type agentKey struct {
	name string
	env  string
}

// agentHistPoint accumulates one timestamp's worth of series data for an agent.
type agentHistPoint struct {
	queueDepth       float64
	itemsProcessed   float64
	itemsFailed      float64
	provisionSeconds float64
}

// SdlcAgents builds the /api/sdlc-agents payload: per-agent, per-env replica
// counts and operational metrics with a 24h/32-sample history. The method adds
// to the Prometheus struct exactly as Fleet() does for /api/fleet.
func (p *Prometheus) SdlcAgents(ctx context.Context) (any, error) {
	const histWindow = 24 * time.Hour
	const histSamples = 32

	byAgent := map[agentKey]*SdlcAgentMetrics{}
	envSet := map[string]bool{}

	get := func(k agentKey) *SdlcAgentMetrics {
		m, ok := byAgent[k]
		if !ok {
			m = &SdlcAgentMetrics{Name: k.name, Env: k.env}
			byAgent[k] = m
		}
		if !envSet[k.env] {
			envSet[k.env] = true
		}
		return m
	}

	// Best-effort sub-query collection (data-architecture.spec section 5.2):
	// collect per-query errors, warn once, still return a usable snapshot.
	var subErrs []error
	var subTotal int
	runInstant := func(expr string, each func(item promResultItem)) {
		subTotal++
		items, err := p.instant(ctx, expr)
		if err != nil {
			subErrs = append(subErrs, err)
			return
		}
		for _, it := range items {
			each(it)
		}
	}

	// Current-value instant queries.
	runInstant(`sum by (name, env) (sdlc_agent)`, func(it promResultItem) {
		k := agentKey{name: it.Metric["name"], env: it.Metric["env"]}
		get(k).ReplicasOn = int(sampleValue(it.Value))
	})
	runInstant(`count by (name, env) (sdlc_agent)`, func(it promResultItem) {
		k := agentKey{name: it.Metric["name"], env: it.Metric["env"]}
		get(k).ReplicasTotal = int(sampleValue(it.Value))
	})
	runInstant(`sum by (name, env) (sdlc_agent_queue_depth)`, func(it promResultItem) {
		k := agentKey{name: it.Metric["name"], env: it.Metric["env"]}
		get(k).QueueDepth = sampleValue(it.Value)
	})
	runInstant(`sum by (name, env) (sdlc_items_processed)`, func(it promResultItem) {
		k := agentKey{name: it.Metric["name"], env: it.Metric["env"]}
		get(k).ItemsProcessed = sampleValue(it.Value)
	})
	runInstant(`sum by (name, env) (sdlc_items_failed)`, func(it promResultItem) {
		k := agentKey{name: it.Metric["name"], env: it.Metric["env"]}
		get(k).ItemsFailed = sampleValue(it.Value)
	})
	// Provision duration is a per-replica gauge; aggregate with max (not sum) so a
	// (name, env) with several replicas reports one representative duration rather
	// than an inflated total.
	runInstant(`max by (name, env) (sdlc_gateway_provision_seconds)`, func(it promResultItem) {
		k := agentKey{name: it.Metric["name"], env: it.Metric["env"]}
		get(k).ProvisionSeconds = sampleValue(it.Value)
	})
	// Freshness fact (SDLC-AGT-15): the per-replica last-push timestamp, aggregated
	// with max so each (name, env) reports its most recent push. Reported as the
	// absolute LastReportedAt (unix seconds); the client judges liveness, not here.
	runInstant(`max by (name, env) (sdlc_agent_last_report_timestamp_seconds)`, func(it promResultItem) {
		k := agentKey{name: it.Metric["name"], env: it.Metric["env"]}
		get(k).LastReportedAt = int64(sampleValue(it.Value))
	})

	// Range queries for the per-agent history series.
	end := time.Now()
	start := end.Add(-histWindow)
	step := histWindow / time.Duration(histSamples)

	// agent -> timestamp -> accumulated sample.
	byTS := map[agentKey]map[int64]*agentHistPoint{}
	pointAt := func(k agentKey, ts int64) *agentHistPoint {
		steps, ok := byTS[k]
		if !ok {
			steps = map[int64]*agentHistPoint{}
			byTS[k] = steps
		}
		pt, ok := steps[ts]
		if !ok {
			pt = &agentHistPoint{}
			steps[ts] = pt
		}
		return pt
	}

	rangeInto := func(expr string, add func(pt *agentHistPoint, v float64)) {
		subTotal++
		res, err := p.queryRange(ctx, expr, start, end, step)
		if err != nil {
			subErrs = append(subErrs, err)
			return
		}
		for _, r := range res {
			k := agentKey{name: r.Metric["name"], env: r.Metric["env"]}
			// Ensure the agent appears in the agent map even if no instant query
			// returned it (a range-only series).
			get(k)
			for _, v := range r.Values {
				add(pointAt(k, sampleTime(v)), sampleValue(v))
			}
		}
	}

	rangeInto(`sum by (name, env) (sdlc_agent_queue_depth)`, func(pt *agentHistPoint, v float64) {
		pt.queueDepth += v
	})
	rangeInto(`sum by (name, env) (sdlc_items_processed)`, func(pt *agentHistPoint, v float64) {
		pt.itemsProcessed += v
	})
	rangeInto(`sum by (name, env) (sdlc_items_failed)`, func(pt *agentHistPoint, v float64) {
		pt.itemsFailed += v
	})
	// max (not sum): one representative provision duration per (name, env), matching
	// the instant query above.
	rangeInto(`max by (name, env) (sdlc_gateway_provision_seconds)`, func(pt *agentHistPoint, v float64) {
		pt.provisionSeconds = v
	})

	// Build the per-agent series aligned to each agent's sorted timestamp axis.
	for k, steps := range byTS {
		tss := make([]int64, 0, len(steps))
		for ts := range steps {
			tss = append(tss, ts)
		}
		sort.Slice(tss, func(i, j int) bool { return tss[i] < tss[j] })

		m := get(k)
		m.Series.Times = tss
		m.Series.QueueDepth = make([]float64, 0, len(tss))
		m.Series.ItemsProcessed = make([]float64, 0, len(tss))
		m.Series.ItemsFailed = make([]float64, 0, len(tss))
		m.Series.ProvisionSeconds = make([]float64, 0, len(tss))
		for _, ts := range tss {
			pt := steps[ts]
			m.Series.QueueDepth = append(m.Series.QueueDepth, pt.queueDepth)
			m.Series.ItemsProcessed = append(m.Series.ItemsProcessed, pt.itemsProcessed)
			m.Series.ItemsFailed = append(m.Series.ItemsFailed, pt.itemsFailed)
			m.Series.ProvisionSeconds = append(m.Series.ProvisionSeconds, pt.provisionSeconds)
		}
	}

	if len(subErrs) > 0 {
		p.logger.WarnContext(ctx, "sdlc-agents snapshot assembled with partial failures",
			"failedSubqueries", len(subErrs),
			"totalSubqueries", subTotal,
			"firstError", subErrs[0].Error())
	}

	// Assemble the payload with non-nil slices.
	envs := make([]string, 0, len(envSet))
	for e := range envSet {
		envs = append(envs, e)
	}
	sort.Strings(envs)

	agents := make([]SdlcAgentMetrics, 0, len(byAgent))
	for _, m := range byAgent {
		// Ensure all series slices are non-nil so JSON produces [] not null.
		if m.Series.Times == nil {
			m.Series.Times = []int64{}
		}
		if m.Series.QueueDepth == nil {
			m.Series.QueueDepth = []float64{}
		}
		if m.Series.ItemsProcessed == nil {
			m.Series.ItemsProcessed = []float64{}
		}
		if m.Series.ItemsFailed == nil {
			m.Series.ItemsFailed = []float64{}
		}
		if m.Series.ProvisionSeconds == nil {
			m.Series.ProvisionSeconds = []float64{}
		}
		agents = append(agents, *m)
	}
	// Stable order: by name, then env.
	sort.Slice(agents, func(i, j int) bool {
		if agents[i].Name != agents[j].Name {
			return agents[i].Name < agents[j].Name
		}
		return agents[i].Env < agents[j].Env
	})

	return SdlcAgentsData{
		Envs:   envs,
		Agents: agents,
	}, nil
}
