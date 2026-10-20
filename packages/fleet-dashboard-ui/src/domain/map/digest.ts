// Middle-truncate a release-bundle digest for display. A digest is long
// (`sha256:` + 64 hex chars) and dominates the compact freight cards and map
// drawer, yet both its head and tail carry signal. We keep the algorithm prefix
// intact and abbreviate the hash to the same 7 characters GitHub shows for a
// short commit SHA, on each side: `sha256:6145e7f...aebd608`.

/** How many hash characters to keep on each side, matching GitHub's short SHA. */
const SHORT = 7;

/**
 * Abbreviate a digest to `sha256:<7>...<7>`. The algorithm prefix (text before
 * the first `:`) is preserved verbatim; only the hash after it is truncated, and
 * only when doing so is actually shorter. A value with no prefix (e.g. the
 * short-SHA fallback identity) is treated as a bare hash. Returns "" for empty.
 */
export function shortDigest(digest: string | null | undefined): string {
  if (!digest) {
    return "";
  }
  const sep = digest.indexOf(":");
  const prefix = sep >= 0 ? digest.slice(0, sep + 1) : "";
  const hash = sep >= 0 ? digest.slice(sep + 1) : digest;
  if (hash.length <= SHORT * 2 + 3) {
    return digest; // already short enough that truncation would not help
  }
  return `${prefix}${hash.slice(0, SHORT)}...${hash.slice(-SHORT)}`;
}
