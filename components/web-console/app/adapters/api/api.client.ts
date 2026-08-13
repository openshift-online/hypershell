import { SDKClient } from "@openshift-online/hypershell-sdk";

export const gatewayCorrelationHeader = "x-hypershell-correlation-id";

export function createCorrelatedFetch(
  correlationId: string,
  fetchImplementation: typeof globalThis.fetch = globalThis.fetch,
): typeof globalThis.fetch {
  return async (input, init) => {
    const headers = new Headers(init?.headers);
    headers.set(gatewayCorrelationHeader, correlationId);
    return await fetchImplementation(input, { ...init, headers });
  };
}

export function createApiClient(correlationId: string): SDKClient {
  return new SDKClient({
    baseUrl: "",
    credentials: "same-origin",
    fetch: createCorrelatedFetch(correlationId),
  });
}
