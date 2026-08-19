import { SDKClient } from "@openshift-online/hypershell-sdk";

export const gatewayCorrelationHeader = "x-hypershell-correlation-id";

/** W3C Trace Context headers a request carries to the BFF for propagation. */
export interface RequestTraceContext {
  traceparent: string;
  tracestate?: string;
}

/**
 * Supplies the W3C trace context for the in-flight workflow, or `undefined`
 * when tracing is disabled or no span is active. Injected so the API adapter
 * stays free of any tracing vendor dependency.
 */
export type TraceContextProvider = () => RequestTraceContext | undefined;

/** The BFF's machine-readable request to restart authentication at the IdP. */
export interface ReauthSignal {
  loginUrl: string;
}

/** Handles a re-authentication signal, typically by navigating to the IdP. */
export type ReauthHandler = (signal: ReauthSignal) => void;

// Guards against firing more than one navigation when several in-flight
// requests receive the re-authentication signal at once.
let reauthInProgress = false;

/**
 * Reads a BFF re-authentication signal from a 401 response, or null when the
 * response is not a re-authentication request. The response body is cloned so
 * the caller can still consume it on the non-reauth path.
 */
async function readReauthSignal(
  response: Response,
): Promise<ReauthSignal | null> {
  if (response.status !== 401) {
    return null;
  }
  if (
    !(response.headers.get("content-type") ?? "").includes("application/json")
  ) {
    return null;
  }
  try {
    const body: unknown = await response.clone().json();
    if (
      typeof body === "object" &&
      body !== null &&
      (body as { error?: unknown }).error === "reauth_required"
    ) {
      const loginUrl = (body as { login_url?: unknown }).login_url;
      return {
        loginUrl: typeof loginUrl === "string" ? loginUrl : "/auth/login",
      };
    }
  } catch {
    // A 401 without a parseable reauth body is not a re-authentication signal.
  }
  return null;
}

/**
 * Full-page navigation to the login endpoint, preserving the current route as
 * `return_to` so the user lands back where they were after re-authenticating.
 * An expired or revoked session is never surfaced as an application error.
 */
export function redirectToLogin(
  signal: ReauthSignal,
  location: Location = globalThis.location,
): void {
  if (reauthInProgress) {
    return;
  }
  reauthInProgress = true;
  const returnTo = `${location.pathname}${location.search}${location.hash}`;
  const target = new URL(signal.loginUrl, location.origin);
  target.searchParams.set("return_to", returnTo);
  location.assign(target.toString());
}

export function createCorrelatedFetch(
  correlationId: string,
  fetchImplementation: typeof globalThis.fetch = globalThis.fetch,
  onReauthRequired?: ReauthHandler,
  traceContext?: TraceContextProvider,
): typeof globalThis.fetch {
  return async (input, init) => {
    const headers = new Headers(init?.headers);
    headers.set(gatewayCorrelationHeader, correlationId);
    const trace = traceContext?.();
    if (trace !== undefined) {
      headers.set("traceparent", trace.traceparent);
      if (trace.tracestate !== undefined && trace.tracestate !== "") {
        headers.set("tracestate", trace.tracestate);
      }
    }
    const response = await fetchImplementation(input, { ...init, headers });
    if (onReauthRequired) {
      const signal = await readReauthSignal(response);
      if (signal) {
        onReauthRequired(signal);
        // The page is navigating to the IdP; never resolve so no downstream
        // error state renders before the redirect completes.
        return await new Promise<Response>(() => {
          /* intentionally never settles */
        });
      }
    }
    return response;
  };
}

export function createApiClient(
  correlationId: string,
  onReauthRequired: ReauthHandler = redirectToLogin,
  traceContext?: TraceContextProvider,
): SDKClient {
  return new SDKClient({
    baseUrl: "",
    credentials: "same-origin",
    fetch: createCorrelatedFetch(
      correlationId,
      globalThis.fetch,
      onReauthRequired,
      traceContext,
    ),
  });
}
