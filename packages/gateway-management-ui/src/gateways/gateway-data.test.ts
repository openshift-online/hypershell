import { describe, expect, it } from "vitest";

import { normalizeGatewayPlacementClusterIds } from "../application/gateway-placement";
import type { GatewayRecord } from "../application/gateway-types";
import {
  gatewayConsoleReadyDeadlineMilliseconds,
  gatewayConsoleUnavailable,
  gatewayNeedsStatusPolling,
  gatewayPlacementBatchQueryKey,
  gatewayStatusPollMilliseconds,
  resolveConsoleWaitStart,
  toGatewayConnection,
} from "./gateway-data";

const CREATED_AT = "2026-08-10T14:30:00Z";
// The console-ready deadline is anchored on when the UI first observed the
// gateway awaiting its console (not its createdAt), so tests supply that
// wait-start explicitly along with a poll time inside the window and one past it.
const CONSOLE_WAIT_START = Date.parse("2026-08-11T09:00:00Z");
const WITHIN_CONSOLE_WINDOW = CONSOLE_WAIT_START + 60_000;
const PAST_CONSOLE_WINDOW =
  CONSOLE_WAIT_START + gatewayConsoleReadyDeadlineMilliseconds + 1;

function gateway(overrides: Partial<GatewayRecord> = {}): GatewayRecord {
  return {
    clusterId: "",
    createdAt: CREATED_AT,
    databaseId: "database-1",
    externalDns: "gateway.example.com",
    id: "gateway-1",
    name: "Team gateway",
    namespace: "openshell",
    phase: "",
    releaseId: "release-1",
    status: "Ready",
    ...overrides,
  };
}

describe("gateway presentation data", () => {
  it("uses one canonical cluster identifier normalization for batch keys", () => {
    const clusterIds = [" cluster-west ", "", "cluster-east", "cluster-west"];

    expect(normalizeGatewayPlacementClusterIds(clusterIds)).toEqual([
      "cluster-east",
      "cluster-west",
    ]);
    expect(gatewayPlacementBatchQueryKey(clusterIds)).toEqual([
      "gateways",
      "placements",
      "batch",
      "cluster-east",
      "cluster-west",
    ]);
  });

  it("maps gateway values into the connection view", () => {
    expect(
      toGatewayConnection(
        gateway({ phase: "Running" }),
        "Localized hub cluster",
      ),
    ).toMatchObject({
      clusterName: "Localized hub cluster",
      createdAt: "2026-08-10T14:30:00Z",
      endpoint: "https://gateway.example.com:443",
      id: "gateway-1",
      name: "Team gateway",
      phase: "Running",
      status: "Ready",
    });
  });

  it("omits phase when the gateway has not reported one", () => {
    expect(
      toGatewayConnection(gateway({ phase: "" }), "Hub cluster").phase,
    ).toBe(undefined);
  });

  it("polls transitional gateway lifecycle states at a bounded interval", () => {
    expect(gatewayStatusPollMilliseconds).toBe(5_000);
    expect(gatewayNeedsStatusPolling(gateway({ phase: "", status: "" }))).toBe(
      true,
    );
    expect(gatewayNeedsStatusPolling(gateway({ phase: "Pending" }))).toBe(true);
    expect(gatewayNeedsStatusPolling(gateway({ phase: "Provisioning" }))).toBe(
      true,
    );
    expect(gatewayNeedsStatusPolling(gateway({ status: "Updating" }))).toBe(
      true,
    );
    expect(gatewayNeedsStatusPolling(gateway({ phase: "Degraded" }))).toBe(
      true,
    );
  });

  it("stops lifecycle polling for terminal gateway states", () => {
    expect(
      gatewayNeedsStatusPolling(
        gateway({
          consoleUrl: "https://console.example.com",
          phase: "Running",
        }),
      ),
    ).toBe(false);
    expect(gatewayNeedsStatusPolling(gateway({ phase: "Failed" }))).toBe(false);
  });

  it("keeps polling a running routed gateway until its console address arrives", () => {
    // A routed gateway reaches Running before its console pod can serve; the
    // control plane publishes console_address only once the pod is Ready. Keep
    // polling so the console button appears without a manual page refresh.
    expect(
      gatewayNeedsStatusPolling(
        gateway({ phase: "Running" }),
        CONSOLE_WAIT_START,
        WITHIN_CONSOLE_WINDOW,
      ),
    ).toBe(true);

    // Once the console URL is published, the button can render and polling stops.
    expect(
      gatewayNeedsStatusPolling(
        gateway({
          consoleUrl: "https://console.example.com",
          phase: "Running",
        }),
        CONSOLE_WAIT_START,
        WITHIN_CONSOLE_WINDOW,
      ),
    ).toBe(false);

    // A non-routed gateway never gains a console, so a settled one does not poll.
    expect(
      gatewayNeedsStatusPolling(
        gateway({ externalDns: undefined, phase: "Running" }),
        CONSOLE_WAIT_START,
        WITHIN_CONSOLE_WINDOW,
      ),
    ).toBe(false);
  });

  it("stops polling and reports the console unavailable past the deadline", () => {
    // A routed gateway can be settled without ever becoming console-eligible
    // (console provisioning disabled/stuck). Polling must not run forever: once
    // the console-ready window elapses, stop polling and surface a terminal
    // "console unavailable" state instead of an indefinite provisioning spinner.
    const settledNoConsole = gateway({ phase: "Running" });

    expect(
      gatewayNeedsStatusPolling(
        settledNoConsole,
        CONSOLE_WAIT_START,
        WITHIN_CONSOLE_WINDOW,
      ),
    ).toBe(true);
    expect(
      gatewayConsoleUnavailable(
        settledNoConsole,
        CONSOLE_WAIT_START,
        WITHIN_CONSOLE_WINDOW,
      ),
    ).toBe(false);

    expect(
      gatewayNeedsStatusPolling(
        settledNoConsole,
        CONSOLE_WAIT_START,
        PAST_CONSOLE_WINDOW,
      ),
    ).toBe(false);
    expect(
      gatewayConsoleUnavailable(
        settledNoConsole,
        CONSOLE_WAIT_START,
        PAST_CONSOLE_WINDOW,
      ),
    ).toBe(true);

    // A still-transitional gateway past the window keeps polling on lifecycle
    // state and is not reported as unavailable.
    const provisioning = gateway({ phase: "Provisioning" });
    expect(
      gatewayNeedsStatusPolling(
        provisioning,
        CONSOLE_WAIT_START,
        PAST_CONSOLE_WINDOW,
      ),
    ).toBe(true);
    expect(
      gatewayConsoleUnavailable(
        provisioning,
        CONSOLE_WAIT_START,
        PAST_CONSOLE_WINDOW,
      ),
    ).toBe(false);

    // A published console is never "unavailable"; a failed gateway surfaces its
    // own failure rather than a console-unavailable state.
    expect(
      gatewayConsoleUnavailable(
        gateway({
          consoleUrl: "https://console.example.com",
          phase: "Running",
        }),
        CONSOLE_WAIT_START,
        PAST_CONSOLE_WINDOW,
      ),
    ).toBe(false);
    expect(
      gatewayConsoleUnavailable(
        gateway({ phase: "Failed" }),
        CONSOLE_WAIT_START,
        PAST_CONSOLE_WINDOW,
      ),
    ).toBe(false);
  });

  it("anchors the console-ready deadline to when console-waiting is first observed", () => {
    // First observation of a settled routed gateway without a console records the
    // wait-start; a gateway created long before it is observed still gets a full
    // window (this is the regression: anchoring on createdAt would mark such a
    // gateway past the deadline the instant it loads and it would never poll).
    const settledNoConsole = gateway({ phase: "Running" });
    const firstSeen = Date.parse(CREATED_AT) + 10 * 60_000;
    expect(
      resolveConsoleWaitStart(settledNoConsole, firstSeen, undefined),
    ).toBe(firstSeen);

    // Subsequent observations keep the original start so the clock is not reset
    // on every poll.
    expect(
      resolveConsoleWaitStart(settledNoConsole, firstSeen + 5_000, firstSeen),
    ).toBe(firstSeen);

    // The gateway is dropped (undefined) once it is no longer awaiting a console,
    // so the caller forgets it and the clock restarts if it becomes eligible
    // again.
    expect(
      resolveConsoleWaitStart(
        gateway({
          consoleUrl: "https://console.example.com",
          phase: "Running",
        }),
        firstSeen,
        firstSeen,
      ),
    ).toBeUndefined();
    expect(
      resolveConsoleWaitStart(
        gateway({ phase: "Provisioning" }),
        firstSeen,
        firstSeen,
      ),
    ).toBeUndefined();
    expect(
      resolveConsoleWaitStart(
        gateway({ phase: "Failed" }),
        firstSeen,
        firstSeen,
      ),
    ).toBeUndefined();
    expect(
      resolveConsoleWaitStart(
        gateway({ externalDns: undefined, phase: "Running" }),
        firstSeen,
        firstSeen,
      ),
    ).toBeUndefined();

    // The recorded start drives the bounded polling: within the window from the
    // wait-start it still polls, even though createdAt is well past.
    expect(
      gatewayNeedsStatusPolling(
        settledNoConsole,
        firstSeen,
        firstSeen + gatewayConsoleReadyDeadlineMilliseconds - 1,
      ),
    ).toBe(true);
    expect(
      gatewayNeedsStatusPolling(
        settledNoConsole,
        firstSeen,
        firstSeen + gatewayConsoleReadyDeadlineMilliseconds + 1,
      ),
    ).toBe(false);
  });

  it("presents transitional and failed lifecycle phases before health", () => {
    expect(
      toGatewayConnection(
        gateway({ phase: "Provisioning", status: "Ready" }),
        "Hub cluster",
      ).status,
    ).toBe("Provisioning");
    expect(
      toGatewayConnection(
        gateway({ phase: "Failed", status: "Ready" }),
        "Hub cluster",
      ).status,
    ).toBe("Failed");
    expect(
      toGatewayConnection(
        gateway({ phase: "Running", status: "Degraded" }),
        "Hub cluster",
      ).status,
    ).toBe("Degraded");
  });

  it("keeps a returned cluster identifier for name resolution only", () => {
    expect(
      toGatewayConnection(
        gateway({ clusterId: "  cluster-east  " }),
        "Hub cluster",
      ),
    ).toMatchObject({
      clusterId: "cluster-east",
      clusterName: "",
    });
  });

  it("keeps API-owned connection values unavailable when they are absent", () => {
    const connection = toGatewayConnection(
      gateway({ externalDns: undefined, status: undefined }),
      "Hub cluster",
    );

    expect(connection.endpoint).toBeUndefined();
    expect(connection.consoleUrl).toBeUndefined();
    expect(connection.oidcIssuer).toBeUndefined();
    expect(connection.status).toBe("Unknown");
  });
});
