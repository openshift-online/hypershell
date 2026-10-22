import type { QuickCreateRandomSource } from "../../application/homepage/quick-create";

export function createBrowserRandomSource(): QuickCreateRandomSource {
  return {
    nextHex(digits) {
      const bytes = new Uint8Array(Math.ceil(digits / 2));
      globalThis.crypto.getRandomValues(bytes);
      return Array.from(bytes, (byte) => byte.toString(16).padStart(2, "0"))
        .join("")
        .slice(0, digits);
    },
    nextInt(maxExclusive) {
      if (maxExclusive <= 0 || maxExclusive > 0x100000000) {
        throw new Error("Random integer range is invalid");
      }
      const candidateLimit =
        Math.floor(0x100000000 / maxExclusive) * maxExclusive;
      for (;;) {
        const bytes = new Uint8Array(4);
        globalThis.crypto.getRandomValues(bytes);
        const value = new DataView(bytes.buffer).getUint32(0);
        if (value < candidateLimit) {
          return value % maxExclusive;
        }
      }
    },
  };
}
