import { afterEach, describe, expect, it, vi } from "vitest";

import {
  queryHomepagePlatformOverview,
  queryHomepageSummary,
} from "../src/homepage.js";

const { gatewayInventoryMock, gatewaySandboxesMock } = vi.hoisted(() => ({
  gatewayInventoryMock: vi.fn(),
  gatewaySandboxesMock: vi.fn(),
}));

vi.mock("../src/authorized-gateway-metrics.js", () => ({
  queryAuthorizedGatewayInventory: gatewayInventoryMock,
}));

vi.mock("../src/metrics-gateway-sandboxes.js", () => ({
  queryGatewaySandboxes: gatewaySandboxesMock,
}));

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    headers: { "content-type": "application/json" },
    status,
  });
}

describe("homepage aggregation", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
    vi.clearAllMocks();
  });

  it("aggregates gateway health", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      jsonResponse({
        items: [
          {
            id: "gateway-1",
            name: "research-gateway",
            phase: "Degraded",
            status: "Connection health is degraded",
          },
          { id: "gateway-2", name: "stable-gateway", phase: "Running" },
        ],
        page: 1,
        total: 2,
      }),
    );
    vi.stubGlobal("fetch", fetchMock);

    await expect(
      queryHomepageSummary("http://api.example.test", "access-token", 100),
    ).resolves.toEqual({
      gatewayHealth: {
        attention: [
          {
            gatewayId: "gateway-1",
            name: "research-gateway",
            reason: "Connection health is degraded",
            status: "Connection health is degraded",
          },
        ],
        attentionCount: 1,
        healthy: 1,
        total: 2,
      },
    });

    expect(fetchMock).toHaveBeenCalledWith(
      new URL(
        "/api/hypershell/v1/gateways?page=1&size=100",
        "http://api.example.test",
      ),
      {
        headers: { authorization: "Bearer access-token" },
        signal: expect.any(AbortSignal) as AbortSignal,
      },
    );
  });

  it("uses local-kind placement for a connected local Kind cluster", async () => {
    const now = new Date().toISOString();
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(
        jsonResponse({
          items: [{ id: "gateway-1", name: "gateway", phase: "Running" }],
          page: 1,
          total: 1,
        }),
      )
      .mockResolvedValueOnce(
        jsonResponse({
          items: [
            {
              capabilities: ["provider/kind", "network/public"],
              id: "cluster-local",
              last_seen_at: now,
              name: "local-kind",
              oidc_subject: "control-plane",
            },
          ],
          page: 1,
          total: 1,
        }),
      );
    vi.stubGlobal("fetch", fetchMock);

    await expect(
      queryHomepageSummary("http://api.example.test", "access-token", 100),
    ).resolves.toEqual({
      gatewayHealth: {
        attention: [],
        attentionCount: 0,
        healthy: 1,
        total: 1,
      },
      quickCreate: { placement: { mode: "local-kind" } },
    });
  });

  it("uses public IBM placement only when a non-Kata candidate is connected", async () => {
    const now = new Date().toISOString();
    const gatewayPage = jsonResponse({
      items: [],
      page: 1,
      total: 0,
    });
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(gatewayPage)
      .mockResolvedValueOnce(
        jsonResponse({
          items: [
            {
              capabilities: ["provider/ibm", "network/public", "runtime/kata"],
              id: "cluster-kata",
              last_seen_at: now,
              name: "ibm-kata",
              oidc_subject: "control-plane",
            },
            {
              capabilities: ["provider/ibm", "network/public"],
              id: "cluster-ibm",
              last_seen_at: now,
              name: "ibm-public",
              oidc_subject: "control-plane",
            },
          ],
          page: 1,
          total: 2,
        }),
      );
    vi.stubGlobal("fetch", fetchMock);

    await expect(
      queryHomepageSummary("http://api.example.test", "access-token", 100),
    ).resolves.toEqual({
      gatewayHealth: {
        attention: [],
        attentionCount: 0,
        healthy: 0,
        total: 0,
      },
      quickCreate: {
        placement: {
          excludedSelector: ["runtime/kata"],
          selector: ["network/public", "provider/ibm"],
        },
      },
    });
  });

  it("omits Quick Create when no eligible cluster is connected", async () => {
    const now = new Date().toISOString();
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(jsonResponse({ items: [], page: 1, total: 0 }))
      .mockResolvedValueOnce(
        jsonResponse({
          items: [
            {
              capabilities: ["provider/ibm", "network/public", "runtime/kata"],
              id: "cluster-kata",
              last_seen_at: now,
              name: "ibm-kata",
              oidc_subject: "control-plane",
            },
          ],
          page: 1,
          total: 1,
        }),
      );
    vi.stubGlobal("fetch", fetchMock);

    await expect(
      queryHomepageSummary("http://api.example.test", "access-token", 100),
    ).resolves.toEqual({
      gatewayHealth: {
        attention: [],
        attentionCount: 0,
        healthy: 0,
        total: 0,
      },
    });
  });

  it("reports gateway health unavailable when the source fails", async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({}, 503));
    vi.stubGlobal("fetch", fetchMock);

    await expect(
      queryHomepageSummary("http://api.example.test", undefined, 100),
    ).resolves.toEqual({
      gatewayHealth: { available: false },
    });
  });

  it("rejects list responses that contain fewer items than the reported total", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      jsonResponse({
        items: [
          {
            id: "gateway-1",
            name: "research-gateway",
            phase: "Running",
          },
        ],
        page: 1,
        total: 2,
      }),
    );
    vi.stubGlobal("fetch", fetchMock);

    await expect(
      queryHomepageSummary("http://api.example.test", "access-token", 100),
    ).resolves.toEqual({
      gatewayHealth: { available: false },
    });
  });

  it("aggregates independently failing platform overview sources", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      jsonResponse({
        items: [
          {
            id: "cluster-1",
            last_seen_at: new Date().toISOString(),
            name: "production-central",
            status: "Ready",
          },
        ],
        page: 1,
        total: 1,
      }),
    );
    vi.stubGlobal("fetch", fetchMock);
    gatewayInventoryMock.mockResolvedValue({
      counts: { Failed: 2, Provisioning: 5, Running: 35 },
      total: 42,
    });
    gatewaySandboxesMock.mockRejectedValue(new Error("Prometheus unavailable"));

    await expect(
      queryHomepagePlatformOverview(
        "http://api.example.test",
        "access-token",
        100,
        "http://prometheus.example.test",
        100,
      ),
    ).resolves.toEqual({
      clusters: {
        healthy: 1,
        total: 1,
        unhealthyClusters: [],
      },
      gatewayStatuses: {
        available: true,
        counts: { Failed: 2, Provisioning: 5, Running: 35 },
        total: 42,
      },
      sandboxStatuses: { available: false },
    });
  });

  it("marks clusters with future heartbeat timestamps as not fresh", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      jsonResponse({
        items: [
          {
            id: "cluster-1",
            last_seen_at: new Date(Date.now() + 10 * 60 * 1000).toISOString(),
            name: "production-central",
            status: "Ready",
          },
        ],
        page: 1,
        total: 1,
      }),
    );
    vi.stubGlobal("fetch", fetchMock);
    gatewayInventoryMock.mockRejectedValue(new Error("Inventory unavailable"));
    gatewaySandboxesMock.mockRejectedValue(new Error("Prometheus unavailable"));

    await expect(
      queryHomepagePlatformOverview(
        "http://api.example.test",
        "access-token",
        100,
        "http://prometheus.example.test",
        100,
      ),
    ).resolves.toEqual({
      clusters: {
        healthy: 0,
        total: 1,
        unhealthyClusters: [
          {
            name: "production-central",
            reason: "Cluster heartbeat is stale",
            status: "Ready",
          },
        ],
      },
      gatewayStatuses: { available: false },
      sandboxStatuses: { available: false },
    });
  });
});
