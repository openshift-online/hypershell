// Pure string helpers over a cluster's self-declared capability set (FLT-CAP-02).
//
// Capabilities are opaque `namespace/value` DATA sourced from delivery labels
// (PLAT-CLUS-06). NOTHING about the fleet's vocabulary is baked in here: an
// unknown namespace or value is ordered and rendered just like a known one, never
// dropped. The only list below is the DISPLAY lead-namespace order - a presentation
// preference over public, product-level namespace names, not a closed vocabulary or
// any fleet identity (data-architecture.spec §3.5 / FLT-MAP-06).

/**
 * Lead namespaces, in the order the UI surfaces them first. Any namespace not in
 * this list sorts after these, alphabetically. This is ordering preference only;
 * an unlisted namespace is kept and shown, never discarded.
 */
const LEAD_NAMESPACES: readonly string[] = [
  "provider",
  "runtime",
  "network",
  "region",
];

/**
 * Split a capability into its namespace and value on the FIRST slash only, so a
 * value that itself contains slashes stays intact. A capability with no slash is treated as a bare value with an empty
 * namespace.
 */
export function capabilityParts(cap: string): {
  namespace: string;
  value: string;
} {
  const slash = cap.indexOf("/");
  if (slash === -1) {
    return { namespace: "", value: cap };
  }
  return { namespace: cap.slice(0, slash), value: cap.slice(slash + 1) };
}

/**
 * The value of the first capability in `namespace`, or null when the set declares
 * none. Matching is on the namespace segment (everything before the first slash).
 */
export function capabilityValue(
  capabilities: readonly string[],
  namespace: string,
): string | null {
  for (const cap of capabilities) {
    const parts = capabilityParts(cap);
    if (parts.namespace === namespace) {
      return parts.value;
    }
  }
  return null;
}

/**
 * Return a new array of the capabilities in stable lead-namespace order: the lead
 * namespaces first (in LEAD_NAMESPACES order), then every other namespace
 * alphabetically. Within one namespace the input order is preserved. No capability
 * is ever dropped - an unknown namespace simply sorts after the lead ones.
 */
export function orderedCapabilities(
  capabilities: readonly string[],
): readonly string[] {
  const rank = (namespace: string): number => {
    const i = LEAD_NAMESPACES.indexOf(namespace);
    // Lead namespaces keep their listed order; everything else shares a single
    // higher rank and is then tie-broken alphabetically below.
    return i === -1 ? LEAD_NAMESPACES.length : i;
  };
  return capabilities
    .map((cap, index) => ({ cap, index, ...capabilityParts(cap) }))
    .sort((a, b) => {
      const ra = rank(a.namespace);
      const rb = rank(b.namespace);
      if (ra !== rb) {
        return ra - rb;
      }
      // Both outside the lead set: alphabetical by namespace, then by value.
      if (ra === LEAD_NAMESPACES.length && a.namespace !== b.namespace) {
        return a.namespace < b.namespace ? -1 : 1;
      }
      // Same namespace (lead or otherwise): keep the input order stable.
      return a.index - b.index;
    })
    .map((e) => e.cap);
}
