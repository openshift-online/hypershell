export interface SdlcAgentSeries {
  readonly times: readonly number[];
  readonly queueDepth: readonly number[];
  readonly itemsProcessed: readonly number[];
  readonly itemsFailed: readonly number[];
  readonly provisionSeconds: readonly number[];
}

export interface SdlcAgentMetrics {
  readonly name: string;
  readonly env: string;
  readonly replicasOn: number;
  readonly replicasTotal: number;
  readonly queueDepth: number;
  readonly itemsProcessed: number;
  readonly itemsFailed: number;
  readonly provisionSeconds: number;
  readonly series: SdlcAgentSeries;
}

export interface SdlcAgentsData {
  readonly envs: readonly string[];
  readonly agents: readonly SdlcAgentMetrics[];
}

/** Group agent rows by environment key (the join key to the map node id). */
export function agentRowsByEnv(
  data: SdlcAgentsData,
): ReadonlyMap<string, readonly SdlcAgentMetrics[]> {
  const byEnv = new Map<string, SdlcAgentMetrics[]>();
  for (const m of data.agents) {
    const rows = byEnv.get(m.env);
    if (rows) rows.push(m);
    else byEnv.set(m.env, [m]);
  }
  return byEnv;
}

/** Sum one series field across rows, aligned to the given (already-merged) axis;
 *  a row missing a sample at a given timestamp contributes 0. */
function sumSeriesAt(
  rows: readonly SdlcAgentMetrics[],
  times: readonly number[],
  pick: (series: SdlcAgentSeries) => readonly number[],
): readonly number[] {
  return times.map((ts) =>
    rows.reduce((total, row) => {
      const idx = row.series.times.indexOf(ts);
      return total + (idx >= 0 ? (pick(row.series)[idx] ?? 0) : 0);
    }, 0),
  );
}

/** Roll up every (name, env) row for one agent name into a single fleet-wide row:
 *  counts sum, and history series merge onto the union of timestamps. */
function sumAgentRows(
  name: string,
  rows: readonly SdlcAgentMetrics[],
): SdlcAgentMetrics {
  const times = [...new Set(rows.flatMap((r) => r.series.times))].sort(
    (a, b) => a - b,
  );
  const sum = (pick: (r: SdlcAgentMetrics) => number): number =>
    rows.reduce((total, r) => total + pick(r), 0);
  return {
    name,
    env: rows.map((r) => r.env).join("+"),
    replicasOn: sum((r) => r.replicasOn),
    replicasTotal: sum((r) => r.replicasTotal),
    queueDepth: sum((r) => r.queueDepth),
    itemsProcessed: sum((r) => r.itemsProcessed),
    itemsFailed: sum((r) => r.itemsFailed),
    // Not a counter: the slowest recent provision across environments is the more
    // useful signal than an average that could mask one environment running hot.
    provisionSeconds: Math.max(...rows.map((r) => r.provisionSeconds)),
    series: {
      times,
      queueDepth: sumSeriesAt(rows, times, (s) => s.queueDepth),
      itemsProcessed: sumSeriesAt(rows, times, (s) => s.itemsProcessed),
      itemsFailed: sumSeriesAt(rows, times, (s) => s.itemsFailed),
      provisionSeconds: sumSeriesAt(rows, times, (s) => s.provisionSeconds),
    },
  };
}

/**
 * Roll up all (name, env) rows for the same agent name into one fleet-wide row,
 * for the Agent SDLC board's per-phase column strip and detail panel - which are
 * env-agnostic (a phase's acting agent is shown once, not once per cluster). A
 * single-env agent passes through unchanged; a multi-env agent (e.g.
 * "spec-review" running in both "integration" and "production") has its scalar
 * counts summed and its history series merged on the union of timestamps, so no
 * environment's telemetry is silently dropped in favor of another.
 */
export function aggregateAgentsByName(
  data: SdlcAgentsData,
): ReadonlyMap<string, SdlcAgentMetrics> {
  const byName = new Map<string, SdlcAgentMetrics[]>();
  for (const m of data.agents) {
    const rows = byName.get(m.name);
    if (rows) rows.push(m);
    else byName.set(m.name, [m]);
  }
  const result = new Map<string, SdlcAgentMetrics>();
  for (const [name, rows] of byName) {
    const [first, ...rest] = rows;
    if (first === undefined) continue; // unreachable: every group has >=1 row
    result.set(name, rest.length === 0 ? first : sumAgentRows(name, rows));
  }
  return result;
}
