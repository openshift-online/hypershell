// FNV-1a 32-bit hash. Pure, deterministic, dependency-free - the seed source for
// the map's identicons and identinames so a given release digest always draws the
// same glyph and reads the same alias. This is a visual hash only; it carries no
// fleet identity and is never used for security.

const FNV_OFFSET_BASIS = 0x811c9dc5;
const FNV_PRIME = 0x01000193;

/**
 * FNV-1a over the UTF-16 code units of `input`, returned as an unsigned 32-bit
 * integer. `Math.imul` keeps the multiply in 32-bit space (matching the classic
 * algorithm); the final `>>> 0` coerces to unsigned.
 */
export function fnv1a(input: string): number {
  let hash = FNV_OFFSET_BASIS;
  for (let i = 0; i < input.length; i++) {
    hash ^= input.charCodeAt(i);
    hash = Math.imul(hash, FNV_PRIME);
  }
  return hash >>> 0;
}
