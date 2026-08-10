import { describe, expect, it } from "vitest";

import {
  parseGatewayListState,
  serializeGatewayListState,
} from "./gateway-list-state";

describe("gateway list URL state", () => {
  it("round-trips the authoritative collection controls", () => {
    const state = parseGatewayListState(
      new URLSearchParams(
        "q=east&page=3&size=50&sort=created&direction=desc&unrelated=ignored",
      ),
    );

    expect(state).toEqual({
      page: 3,
      search: "east",
      size: 50,
      sortDirection: "desc",
      sortField: "created",
    });
    expect(serializeGatewayListState(state).toString()).toBe(
      "q=east&page=3&size=50&sort=created&direction=desc",
    );
  });

  it("normalizes invalid controls to stable defaults", () => {
    expect(
      parseGatewayListState(
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
