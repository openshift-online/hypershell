import { act, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

// The store holds module-level state and is intentionally one-way (once expired, it stays
// expired until a full page navigation). We reset the module between cases to start clean,
// then exercise it through its public surface (the useSessionExpired hook).
async function freshStore() {
  vi.resetModules();
  return import("./session-expiry");
}

describe("session-expiry store", () => {
  afterEach(() => {
    vi.resetModules();
  });

  it("starts un-expired", async () => {
    const { useSessionExpired } = await freshStore();
    const { result } = renderHook(() => useSessionExpired());
    expect(result.current).toBe(false);
  });

  it("flips subscribed consumers to expired on mark", async () => {
    const { useSessionExpired, markSessionExpired } = await freshStore();
    const { result } = renderHook(() => useSessionExpired());

    act(() => {
      markSessionExpired();
    });

    expect(result.current).toBe(true);
  });

  it("is idempotent: a second mark is a no-op", async () => {
    const { useSessionExpired, markSessionExpired } = await freshStore();
    const { result } = renderHook(() => useSessionExpired());

    act(() => {
      markSessionExpired();
      markSessionExpired();
    });

    expect(result.current).toBe(true);
  });
});
