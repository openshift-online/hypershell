// Query-key factory. Central so every hook and any invalidation share one source
// of truth for cache keys.

export const queryKeys = {
  fleet: ["fleet"] as const,
  promotion: ["promotion"] as const,
  topology: ["topology"] as const,
  instances: ["instances"] as const,
};
