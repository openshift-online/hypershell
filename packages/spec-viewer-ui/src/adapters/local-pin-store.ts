import type { PinStorePort } from "../application/ports";

const STORAGE_KEY = "hypershell-spec-viewer.pins";

export function createLocalPinStore(
  storage: Storage | undefined,
): PinStorePort {
  return {
    load() {
      try {
        const stored: unknown = JSON.parse(
          storage?.getItem(STORAGE_KEY) ?? "[]",
        );
        return Array.isArray(stored)
          ? stored.filter((id): id is string => typeof id === "string")
          : [];
      } catch {
        // Unreadable or blocked storage starts with no pins.
        return [];
      }
    },
    save(nodeIds) {
      try {
        storage?.setItem(STORAGE_KEY, JSON.stringify(nodeIds));
      } catch {
        // Pins stay in memory for this session when storage is blocked.
      }
    },
  };
}
