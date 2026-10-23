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

/** Return the latest sample from a series, or 0 if empty. */
export function latestSample(series: readonly number[]): number {
  return series.at(-1) ?? 0;
}
