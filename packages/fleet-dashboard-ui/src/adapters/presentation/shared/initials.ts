// Bundle identicons drawn with the DiceBear Initials style (FLT-DLV-24). Every
// tab draws a bundle with the same picture. The letters come from the bundle's
// alias (itself derived from the digest), so "hazy basil" reads "HB"; seeding
// with the raw digest would give every bundle the same "S". The style
// definition ships with the app; nothing is fetched at runtime.

import { Avatar, Style } from "@dicebear/core";
import initials from "@dicebear/styles/initials.json";
import { identiName } from "../../../domain/map/identiname";

const style = new Style(initials);
const cache = new Map<string, string>();

/** A data URI of the Initials identicon for the bundle `seed`, cached per seed. */
export function initialsDataUri(seed: string): string {
  const cached = cache.get(seed);
  if (cached !== undefined) return cached;
  const uri = new Avatar(style, { seed: identiName(seed) }).toDataUri();
  cache.set(seed, uri);
  return uri;
}
