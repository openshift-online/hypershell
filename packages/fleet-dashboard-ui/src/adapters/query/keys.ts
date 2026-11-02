// Query-key factory. Central so every hook and any invalidation share one source
// of truth for cache keys.

export const queryKeys = {
  fleet: ["fleet"] as const,
  promotion: ["promotion"] as const,
  topology: ["topology"] as const,
  instances: ["instances"] as const,
  sdlc: ["sdlc"] as const,
  sdlcAgents: ["sdlc-agents"] as const,
  e2eRuns: ["e2e", "runs"] as const,
  e2eRun: (id: string) => ["e2e", "run", id] as const,
};
