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

  it("signInAgain navigates to the oauth-proxy sign-out so the stale cookie is cleared", async () => {
    // A reload would leave the still-valid proxy cookie in place and loop straight back to 401;
    // recovery has to route through sign_out (which clears the cookie) and return to the app.
    // jsdom's window.location.assign is non-configurable, so swap the whole location object.
    const original = window.location;
    const assign = vi.fn();
    Object.defineProperty(window, "location", {
      configurable: true,
      value: { assign },
    });
    try {
      const { signInAgain } = await freshStore();
      signInAgain();
      expect(assign).toHaveBeenCalledWith("/oauth2/sign_out?rd=/");
    } finally {
      Object.defineProperty(window, "location", {
        configurable: true,
        value: original,
      });
    }
  });
});
