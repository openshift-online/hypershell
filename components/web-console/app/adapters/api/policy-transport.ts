import { SDKAPIError } from "@openshift-online/hypershell-sdk";

/*
 * JSON transport for the platform policy endpoints the generated SDK does not
 * model (evaluate, findings, the settings singleton, and a gateway's effective
 * policies). It uses the same correlated, trace-propagating fetch as the SDK
 * client and raises the SDK's error type so adapters map failures identically.
 */

export type PolicyRequestMethod = "DELETE" | "GET" | "PATCH" | "POST";

export interface PolicyTransport {
  request(
    method: PolicyRequestMethod,
    path: string,
    options?: { body?: unknown; signal?: AbortSignal },
  ): Promise<unknown>;
}

export type PolicyTransportFactory = (correlationId: string) => PolicyTransport;

function text(value: unknown, fallback: string): string {
  return typeof value === "string" ? value : fallback;
}

async function apiError(response: Response): Promise<SDKAPIError> {
  let body: Record<string, unknown> = {};
  try {
    const parsed: unknown = await response.json();
    if (typeof parsed === "object" && parsed !== null) {
      body = parsed as Record<string, unknown>;
    }
  } catch {
    // A non-JSON error body still maps by status code.
  }
  return new SDKAPIError({
    code: text(body.code, "unknown_error"),
    href: text(body.href, ""),
    id: text(body.id, ""),
    kind: text(body.kind, "Error"),
    operation_id: text(body.operation_id, ""),
    reason: text(body.reason, `HTTP ${String(response.status)}`),
    status_code: response.status,
  });
}

export function createPolicyTransport(
  fetchImplementation: typeof globalThis.fetch,
): PolicyTransport {
  return {
    async request(method, path, options = {}) {
      const headers: Record<string, string> = {};
      if (options.body !== undefined) {
        headers["Content-Type"] = "application/json";
      }
      const response = await fetchImplementation(path, {
        body:
          options.body === undefined ? undefined : JSON.stringify(options.body),
        credentials: "same-origin",
        headers,
        method,
        signal: options.signal,
      });
      if (!response.ok) {
        throw await apiError(response);
      }
      if (response.status === 204) {
        return undefined;
      }
      return (await response.json()) as unknown;
    },
  };
}
