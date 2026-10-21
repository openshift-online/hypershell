export interface HomepageAttentionEntry {
  gatewayId: string;
  name: string;
  reason: string;
  status: string;
}

export interface HomepageSummary {
  gatewayHealth:
    | {
        attention: readonly HomepageAttentionEntry[];
        attentionCount: number;
        healthy: number;
        total: number;
      }
    | { available: false };
  supportChannel?: {
    channelLabel: string;
    description: string;
    title: string;
    url: string;
  };
}

export interface HomepagePlatformOverview {
  clusters:
    | {
        healthy: number;
        total: number;
        unhealthyClusters: readonly {
          name: string;
          reason: string;
          status: string;
        }[];
      }
    | { available: false };
  gatewayStatuses:
    | { available: true; counts: Record<string, number>; total: number }
    | { available: false };
  sandboxStatuses:
    | { available: true; counts: Record<string, number>; total: number }
    | { available: false };
}

export interface HomepageGateway {
  getHomepagePlatformOverview(
    signal?: AbortSignal,
  ): Promise<HomepagePlatformOverview>;
  getHomepageSummary(signal?: AbortSignal): Promise<HomepageSummary>;
}

async function requestJson<T>(
  pathname: string,
  signal: AbortSignal | undefined,
  fetchImplementation: typeof globalThis.fetch,
): Promise<T> {
  const response = await fetchImplementation(pathname, {
    credentials: "same-origin",
    headers: { accept: "application/json" },
    ...(signal ? { signal } : {}),
  });
  if (!response.ok) {
    throw new Error(`Homepage request failed: ${String(response.status)}`);
  }
  return (await response.json()) as T;
}

export function createHomepageAdapter(
  fetchImplementation: typeof globalThis.fetch = globalThis.fetch,
): HomepageGateway {
  return {
    getHomepagePlatformOverview: (signal) =>
      requestJson<HomepagePlatformOverview>(
        "/api/homepage/platform-overview",
        signal,
        fetchImplementation,
      ),
    getHomepageSummary: (signal) =>
      requestJson<HomepageSummary>(
        "/api/homepage/summary",
        signal,
        fetchImplementation,
      ),
  };
}
