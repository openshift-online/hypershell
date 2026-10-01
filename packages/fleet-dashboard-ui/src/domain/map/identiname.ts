// Deterministic 2-word alias ("identiname") paired with the map's identicons so
// a given seed always produces the same human-readable label. Capitalisation and
// truncation are presentation concerns; this layer returns raw lowercase.

import { fnv1a } from "./hash";

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

/** Stable "adjective noun" alias for `seed`, e.g. `"amber pickle"`. */
export function identiName(seed: string): string {
  const adj = ADJ[fnv1a("adj:" + seed) % ADJ.length] ?? "";
  const noun = NOUN[fnv1a("noun:" + seed) % NOUN.length] ?? "";
  return `${adj} ${noun}`;
}
