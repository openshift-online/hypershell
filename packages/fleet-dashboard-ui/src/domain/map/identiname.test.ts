import { describe, expect, it } from "vitest";

import { fnv1a } from "./hash";
import { identiName } from "./identiname";

// Mirror the module-local lists so the spot-check can re-derive the expected
// result from the algorithm rather than hard-coding a string literal.
const ADJ = [
  "amber",
  "anguished",
  "brisk",
  "copper",
  "dapper",
  "eager",
  "frosty",
  "gilded",
  "hazy",
  "ionic",
  "jovial",
  "keen",
  "lucid",
  "mellow",
  "nimble",
  "opal",
  "plucky",
  "quiet",
  "rustic",
  "savvy",
  "teal",
  "umber",
  "vivid",
  "witty",
  "zesty",
  "azure",
  "bramble",
  "candid",
  "dusky",
  "feral",
  "glassy",
  "hushed",
] as const;

const NOUN = [
  "pickle",
  "otter",
  "comet",
  "harbor",
  "lantern",
  "maple",
  "nimbus",
  "quartz",
  "raven",
  "sequoia",
  "tundra",
  "vortex",
  "willow",
  "yarrow",
  "zephyr",
  "basil",
  "cinder",
  "dune",
  "ember",
  "fjord",
  "gecko",
  "heron",
  "kelp",
  "larch",
  "marble",
  "nettle",
  "onyx",
  "petal",
  "reed",
  "sable",
  "thorn",
  "vellum",
] as const;

describe("identiName", () => {
  it("is deterministic for the same seed", () => {
    const seeds = ["hypershell", "alpha-01", "spoke-eu-west"];
    for (const seed of seeds) {
      expect(identiName(seed)).toBe(identiName(seed));
    }
  });

  it("returns two lowercase words separated by a space", () => {
    const result = identiName("test-seed");
    expect(result).toMatch(/^[a-z]+ [a-z]+$/);
    expect(result.split(" ")).toHaveLength(2);
  });

  it("returns a first word from ADJ and a second word from NOUN", () => {
    const [adj, noun] = identiName("fleet-node").split(" ");
    expect(ADJ).toContain(adj);
    expect(NOUN).toContain(noun);
  });

  it("produces distinct names for different seeds", () => {
    const seeds = ["a", "b", "c", "d", "e", "f", "g", "h"];
    const names = seeds.map((s) => identiName(s));
    const unique = new Set(names);
    expect(unique.size).toBeGreaterThan(1);
  });

  it("does not throw on an empty-string seed and returns a valid name", () => {
    const result = identiName("");
    expect(result).toMatch(/^[a-z]+ [a-z]+$/);
    expect(result.split(" ")).toHaveLength(2);
  });

  it("maps a known seed to the expected adj+noun via the algorithm", () => {
    const seed = "hypershell";
    const adjIndex = fnv1a("adj:" + seed) % ADJ.length;
    const nounIndex = fnv1a("noun:" + seed) % NOUN.length;
    const expectedAdj = ADJ[adjIndex] ?? "";
    const expectedNoun = NOUN[nounIndex] ?? "";
    expect(identiName(seed)).toBe(`${expectedAdj} ${expectedNoun}`);
  });
});
