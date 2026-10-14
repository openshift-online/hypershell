import { describe, expect, it } from "vitest";

import { sortUserUsage, userUsageTotals, type UserUsage } from "./users";

const USERS: readonly UserUsage[] = [
  {
    username: "alice",
    gateways: 7,
    sandboxes: 64,
    clusters: ["euwest-1", "useast-1"],
  },
  { username: "bob", gateways: 1, sandboxes: 0, clusters: ["useast-1"] },
  { username: "carol", gateways: 0, sandboxes: 0, clusters: [] },
];

describe("userUsageTotals", () => {
  it("counts distinct users and owners separately from raw sums", () => {
    expect(userUsageTotals(USERS)).toEqual({
      users: 3,
      owners: 2,
      gateways: 8,
      sandboxes: 64,
    });
  });

  it("returns zeros for an empty plane", () => {
    expect(userUsageTotals([])).toEqual({
      users: 0,
      owners: 0,
      gateways: 0,
      sandboxes: 0,
    });
  });
});

describe("sortUserUsage", () => {
  it("sorts by gateway count descending with a username tiebreak", () => {
    const sorted = sortUserUsage(USERS, "gateways", "desc");
    expect(sorted.map((u) => u.username)).toEqual(["alice", "bob", "carol"]);
  });

  it("breaks count ties by username so ordering is stable", () => {
    const tied: readonly UserUsage[] = [
      { username: "zoe", gateways: 2, sandboxes: 1, clusters: [] },
      { username: "amy", gateways: 2, sandboxes: 5, clusters: [] },
    ];
    const sorted = sortUserUsage(tied, "gateways", "desc");
    expect(sorted.map((u) => u.username)).toEqual(["amy", "zoe"]);
  });

  it("sorts by sandboxes ascending", () => {
    const sorted = sortUserUsage(USERS, "sandboxes", "asc");
    expect(sorted.map((u) => u.username)).toEqual(["bob", "carol", "alice"]);
  });

  it("sorts by username ascending lexicographically", () => {
    const sorted = sortUserUsage(USERS, "username", "asc");
    expect(sorted.map((u) => u.username)).toEqual(["alice", "bob", "carol"]);
  });

  it("does not mutate the input list", () => {
    const input: UserUsage[] = [
      { username: "b", gateways: 2, sandboxes: 0, clusters: [] },
      { username: "a", gateways: 1, sandboxes: 0, clusters: [] },
    ];
    sortUserUsage(input, "gateways", "desc");
    expect(input.map((u) => u.username)).toEqual(["b", "a"]);
  });
});
