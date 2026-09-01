import { describe, expect, it } from "vitest";

import {
  parseGatewayProfileListState,
  serializeGatewayProfileListState,
} from "./gateway-profile-list-state";

describe("gateway profile list URL state", () => {
  it("round-trips the authoritative collection controls", () => {
    const state = parseGatewayProfileListState(
      new URLSearchParams(
        "q=small&page=3&size=50&sort=created&direction=desc&unrelated=ignored",
      ),
    );

    expect(state).toEqual({
      page: 3,
      search: "small",
      size: 50,
      sortDirection: "desc",
      sortField: "created",
    });
    expect(serializeGatewayProfileListState(state).toString()).toBe(
      "q=small&page=3&size=50&sort=created&direction=desc",
    );
  });

  it("normalizes invalid controls to stable defaults", () => {
    expect(
      parseGatewayProfileListState(
        new URLSearchParams("page=-1&size=999&sort=made-up&direction=sideways"),
      ),
    ).toEqual({
      page: 1,
      search: "",
      size: 20,
      sortDirection: "asc",
      sortField: "name",
    });
  });
});
