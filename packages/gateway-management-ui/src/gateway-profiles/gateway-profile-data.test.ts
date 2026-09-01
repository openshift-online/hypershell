import { describe, expect, it } from "vitest";

import { defaultGatewayProfileListRequest } from "../application/gateway-profile-types";
import {
  gatewayProfileListQueryKey,
  gatewayProfileListQueryRoot,
  gatewayProfileQueryKey,
  gatewayProfileSearchQueryKey,
} from "./gateway-profile-data";

describe("gateway profile query keys", () => {
  it("builds a stable list query key from the request", () => {
    expect(
      gatewayProfileListQueryKey(defaultGatewayProfileListRequest),
    ).toEqual([...gatewayProfileListQueryRoot, 1, 20, "", "name", "asc"]);
  });

  it("distinguishes list keys by page, size, search, and sort", () => {
    const first = gatewayProfileListQueryKey({
      page: 2,
      search: "small",
      size: 50,
      sortDirection: "desc",
      sortField: "created",
    });
    const second = gatewayProfileListQueryKey(defaultGatewayProfileListRequest);

    expect(first).not.toEqual(second);
  });

  it("builds a detail query key", () => {
    expect(gatewayProfileQueryKey("profile-1")).toEqual([
      "gateway-profiles",
      "detail",
      "profile-1",
    ]);
  });

  it("trims the search query key", () => {
    expect(gatewayProfileSearchQueryKey("  small  ")).toEqual([
      "gateway-profiles",
      "search",
      "small",
    ]);
  });
});
