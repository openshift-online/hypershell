import { describe, expect, it } from "vitest";

import {
  parseGatewayListState,
  serializeGatewayListState,
} from "./gateway-list-state";

describe("gateway list URL state", () => {
  it("round-trips the authoritative collection controls", () => {
    const state = parseGatewayListState(
      new URLSearchParams(
        "q=east&page=3&size=50&sort=owner&direction=asc&owner=mine&unrelated=ignored",
      ),
    );

    expect(state).toEqual({
      onlyMine: true,
      page: 3,
      search: "east",
      size: 50,
      sortDirection: "asc",
      sortField: "owner",
    });
    expect(serializeGatewayListState(state).toString()).toBe(
      "q=east&page=3&size=50&sort=owner&direction=asc&owner=mine",
    );
  });

  it("normalizes invalid controls to stable defaults (newest-first)", () => {
    expect(
      parseGatewayListState(
        new URLSearchParams("page=-1&size=999&sort=made-up&direction=sideways"),
      ),
    ).toEqual({
      onlyMine: false,
      page: 1,
      search: "",
      size: 20,
      sortDirection: "desc",
      sortField: "created",
    });
  });

  it("omits the default newest-first sort and the off ownership filter", () => {
    const state = parseGatewayListState(new URLSearchParams(""));

    expect(state).toEqual({
      onlyMine: false,
      page: 1,
      search: "",
      size: 20,
      sortDirection: "desc",
      sortField: "created",
    });
    // Default order and an unset ownership filter produce a clean URL.
    expect(serializeGatewayListState(state).toString()).toBe("");
  });

  it("round-trips the active sandbox count sort", () => {
    const state = parseGatewayListState(
      new URLSearchParams("sort=activeSandboxes&direction=desc"),
    );

    expect(state).toEqual({
      onlyMine: false,
      page: 1,
      search: "",
      size: 20,
      sortDirection: "desc",
      sortField: "activeSandboxes",
    });
    // direction=desc is the default and is omitted from the serialized URL.
    expect(serializeGatewayListState(state).toString()).toBe(
      "sort=activeSandboxes",
    );
  });
});
