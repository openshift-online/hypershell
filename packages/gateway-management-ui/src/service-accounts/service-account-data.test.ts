import { describe, expect, it } from "vitest";

import { defaultOpenShellGatewayServiceAccountListRequest } from "../application/gateway-types";
import {
  serviceAccountListQueryKey,
  serviceAccountListQueryRoot,
  serviceAccountNeedsPolling,
  serviceAccountQueryKey,
} from "./service-account-data";

describe("service-account query state", () => {
  it.each(["provisioning", "revoking", "deleting"] as const)(
    "polls the collection for %s",
    (status) => {
      expect(serviceAccountNeedsPolling({ status })).toBe(true);
    },
  );

  it.each(["ready", "revoked", "expired", "error"] as const)(
    "stops polling for %s",
    (status) => {
      expect(serviceAccountNeedsPolling({ status })).toBe(false);
    },
  );

  it("scopes list and detail keys to one gateway", () => {
    expect(
      serviceAccountListQueryKey("gateway-1", {
        ...defaultOpenShellGatewayServiceAccountListRequest,
        search: "bot",
        status: "ready",
      }),
    ).toEqual([
      "gateways",
      "detail",
      "gateway-1",
      "service-accounts",
      "list",
      1,
      20,
      "bot",
      "ready",
      "created_at",
      "desc",
    ]);
    expect(serviceAccountListQueryRoot("gateway-1")).toEqual([
      "gateways",
      "detail",
      "gateway-1",
      "service-accounts",
      "list",
    ]);
    expect(serviceAccountQueryKey("gateway-1", "account-1")).toContain(
      "account-1",
    );
  });
});
