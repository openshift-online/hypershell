import { describe, expect, it, vi } from "vitest";

import {
  createCorrelatedFetch,
  gatewayCorrelationHeader,
  redirectToLogin,
} from "./api.client";

const reauthResponse = () =>
  new Response(
    JSON.stringify({ error: "reauth_required", login_url: "/auth/login" }),
    { headers: { "content-type": "application/json" }, status: 401 },
  );

describe("correlated API fetch", () => {
  it("adds the application invocation correlation identifier", async () => {
    const fetchImplementation = vi
      .fn<typeof globalThis.fetch>()
      .mockResolvedValue(new Response(null, { status: 204 }));
    const correlatedFetch = createCorrelatedFetch(
      "11111111-1111-4111-8111-111111111111",
      fetchImplementation,
    );

    await correlatedFetch("/api/hypershell/v1/gateways", {
      headers: { accept: "application/json" },
    });

    expect(fetchImplementation).toHaveBeenCalledOnce();
    const init = fetchImplementation.mock.calls[0]?.[1];
    const headers = new Headers(init?.headers);
    expect(headers.get("accept")).toBe("application/json");
    expect(headers.get(gatewayCorrelationHeader)).toBe(
      "11111111-1111-4111-8111-111111111111",
    );
  });

  it("invokes the re-authentication handler on a reauth_required 401", async () => {
    const onReauth = vi.fn();
    const fetchImplementation = vi
      .fn<typeof globalThis.fetch>()
      .mockResolvedValue(reauthResponse());
    const correlatedFetch = createCorrelatedFetch(
      "22222222-2222-4222-8222-222222222222",
      fetchImplementation,
      onReauth,
    );

    // The returned promise never settles once a redirect is initiated, so the
    // call is fired without awaiting it.
    void correlatedFetch("/api/hypershell/v1/gateways");

    await vi.waitFor(() => {
      expect(onReauth).toHaveBeenCalledWith({ loginUrl: "/auth/login" });
    });
  });

  it("passes through a 401 that is not a re-authentication signal", async () => {
    const onReauth = vi.fn();
    const fetchImplementation = vi
      .fn<typeof globalThis.fetch>()
      .mockResolvedValue(
        new Response(JSON.stringify({ error: "forbidden" }), {
          headers: { "content-type": "application/json" },
          status: 401,
        }),
      );
    const correlatedFetch = createCorrelatedFetch(
      "33333333-3333-4333-8333-333333333333",
      fetchImplementation,
      onReauth,
    );

    const response = await correlatedFetch("/api/hypershell/v1/gateways");

    expect(onReauth).not.toHaveBeenCalled();
    expect(response.status).toBe(401);
  });

  it("propagates the W3C trace context supplied by the provider", async () => {
    const fetchImplementation = vi
      .fn<typeof globalThis.fetch>()
      .mockResolvedValue(new Response(null, { status: 204 }));
    const correlatedFetch = createCorrelatedFetch(
      "44444444-4444-4444-8444-444444444444",
      fetchImplementation,
      undefined,
      () => ({
        traceparent:
          "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01",
        tracestate: "hypershell=1",
      }),
    );

    await correlatedFetch("/api/hypershell/v1/gateways");

    const headers = new Headers(fetchImplementation.mock.calls[0]?.[1]?.headers);
    expect(headers.get("traceparent")).toBe(
      "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01",
    );
    expect(headers.get("tracestate")).toBe("hypershell=1");
  });

  it("omits trace headers when the provider reports no active span", async () => {
    const fetchImplementation = vi
      .fn<typeof globalThis.fetch>()
      .mockResolvedValue(new Response(null, { status: 204 }));
    const correlatedFetch = createCorrelatedFetch(
      "55555555-5555-4555-8555-555555555555",
      fetchImplementation,
      undefined,
      () => undefined,
    );

    await correlatedFetch("/api/hypershell/v1/gateways");

    const headers = new Headers(fetchImplementation.mock.calls[0]?.[1]?.headers);
    expect(headers.has("traceparent")).toBe(false);
    expect(headers.has("tracestate")).toBe(false);
  });
});

describe("redirectToLogin", () => {
  it("navigates to the login endpoint preserving the current route once", () => {
    const assign = vi.fn();
    const location = {
      assign,
      hash: "",
      origin: "https://console.example.test",
      pathname: "/gateways/gw-1",
      search: "?tab=details",
    } as unknown as Location;

    redirectToLogin({ loginUrl: "/auth/login" }, location);
    // A second signal must not trigger a second navigation.
    redirectToLogin({ loginUrl: "/auth/login" }, location);

    expect(assign).toHaveBeenCalledOnce();
    const target = new URL(assign.mock.calls[0]?.[0] as string);
    expect(target.pathname).toBe("/auth/login");
    expect(target.searchParams.get("return_to")).toBe(
      "/gateways/gw-1?tab=details",
    );
  });
});
