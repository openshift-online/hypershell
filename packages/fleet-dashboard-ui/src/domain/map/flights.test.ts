import { describe, expect, it } from "vitest";

import { computeFlights } from "./flights";
import type { MapColumn, MapModel, MapNode } from "./model";

// Minimal MapNode: computeFlights only reads id, seed and columnKey.
function node(id: string, columnKey: string, seed: string): MapNode {
  return {
    id,
    columnKey,
    envLabel: null,
    laneKey: "none",
    isHub: true,
    gateNames: [],
    gateChecks: [],
    role: null,
    provider: null,
    cluster: null,
    seed,
    version: null,
    digest: null,
    proposedVersion: null,
    proposedDigest: null,
    state: "behind",
    upToDate: false,
    driftsFromColumn: false,
    argoHealth: null,
    argoSync: null,
    prState: null,
    gateBadge: { tone: "unknown", labelKey: "unknown" },
    gateways: {},
    gatewaysTotal: 0,
    gatewaysByCluster: [],
    spokeAttribution: null,
    gatewayTone: "unknown",
    gatewayHistory: [],
    sandboxes: 0,
    sandboxesByCluster: [],
    sandboxHistory: [],
    logins: null,
    userHistory: [],
    loginsHistory: [],
    historyTimes: [],
    managedClusters: null,
    users: null,
    metrics: {
      rpc: { rate: 0, errorPct: 0, p95Ms: 0 },
      reconcile: { rate: 0, errorPct: 0, p95Ms: 0 },
      bff: { rate: 0, errorPct: 0, p95Ms: 0 },
      provisionP95Ms: null,
    },
    links: {
      console: null,
      grafana: null,
      argo: null,
      pr: null,
      analysis: null,
    },
  };
}

// Three columns in promotion order; nodeIds are unused by computeFlights.
function model(nodes: MapNode[]): MapModel {
  const keys = ["int", "stage", "prod"];
  const columns: MapColumn[] = keys.map((key, index) => ({
    key,
    index,
    nodeIds: nodes.filter((n) => n.columnKey === key).map((n) => n.id),
    envLabel: key,
  }));
  return { columns, lanes: [], nodes, gates: [], frontier: null };
}

describe("computeFlights", () => {
  it("returns nothing on first render (no prior model)", () => {
    const next = model([node("a", "int", "seed-1")]);
    expect(computeFlights(null, next)).toEqual([]);
  });

  it("returns nothing when no seed changed", () => {
    const prev = model([
      node("int", "int", "s1"),
      node("stage", "stage", "s0"),
    ]);
    const next = model([
      node("int", "int", "s1"),
      node("stage", "stage", "s0"),
    ]);
    expect(computeFlights(prev, next)).toEqual([]);
  });

  it("flies a bundle from the upstream node it was running on", () => {
    // s1 was on int; next tick it lands on stage -> flight int -> stage.
    const prev = model([
      node("int", "int", "s1"),
      node("stage", "stage", "s0"),
    ]);
    const next = model([
      node("int", "int", "s2"),
      node("stage", "stage", "s1"),
    ]);
    const flights = computeFlights(prev, next);
    expect(flights).toContainEqual({
      sourceId: "int",
      destId: "stage",
      seed: "s1",
      destPrevSeed: "s0",
    });
    // int simultaneously receives a brand-new bundle from the tray (full chain).
    expect(flights).toContainEqual({
      sourceId: "",
      destId: "int",
      seed: "s2",
      destPrevSeed: "s1",
      fromTray: true,
    });
  });

  it("chooses the NEAREST upstream source for the landed bundle", () => {
    // s1 sat on both int and stage before; prod lands it -> source is stage (nearer).
    const prev = model([
      node("int", "int", "s1"),
      node("stage", "stage", "s1"),
      node("prod", "prod", "s0"),
    ]);
    const next = model([
      node("int", "int", "s2"),
      node("stage", "stage", "s2"),
      node("prod", "prod", "s1"),
    ]);
    const flights = computeFlights(prev, next);
    expect(flights).toContainEqual({
      sourceId: "stage",
      destId: "prod",
      seed: "s1",
      destPrevSeed: "s0",
    });
    expect(flights.find((fl) => fl.destId === "prod")?.sourceId).toBe("stage");
  });

  it("flies a brand-new first-column bundle in FROM THE TRAY", () => {
    // int gets a brand-new frontier bundle no other node held: it flew in from the
    // release tray (completes the chain), marked fromTray with no source node.
    const prev = model([node("int", "int", "s1")]);
    const next = model([node("int", "int", "s2")]);
    expect(computeFlights(prev, next)).toEqual([
      {
        sourceId: "",
        destId: "int",
        seed: "s2",
        destPrevSeed: "s1",
        fromTray: true,
      },
    ]);
  });

  it("does not fly a downstream bundle that has no upstream origin", () => {
    // A non-first-column node whose new seed no upstream node held is NOT a tray arrival
    // (the tray only feeds the first column) -> no flight.
    const prev = model([
      node("int", "int", "s1"),
      node("stage", "stage", "s0"),
    ]);
    const next = model([
      node("int", "int", "s1"),
      node("stage", "stage", "x9"),
    ]);
    expect(computeFlights(prev, next)).toEqual([]);
  });

  it("ignores an empty landed seed", () => {
    const prev = model([
      node("int", "int", "s1"),
      node("stage", "stage", "s0"),
    ]);
    const next = model([node("int", "int", "s1"), node("stage", "stage", "")]);
    expect(computeFlights(prev, next)).toEqual([]);
  });
});
