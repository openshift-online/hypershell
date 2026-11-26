// Bundle identicons with the DiceBear Initials style (FLT-DLV-24): the letters
// of the bundle's alias, the same picture on every tab. The definition ships
// with the bundle.
import { Avatar, Style } from "@dicebear/core";
import initials from "@dicebear/styles/initials.json";

const style = new Style(initials);
const cache = new Map<string, string>();

/** The Initials identicon for a bundle alias, e.g. "hazy basil" reads "HB". */
export function initialsDataUri(alias: string): string {
  const cached = cache.get(alias);
  if (cached !== undefined) return cached;
  const uri = new Avatar(style, { seed: alias }).toDataUri();
  cache.set(alias, uri);
  return uri;
}
