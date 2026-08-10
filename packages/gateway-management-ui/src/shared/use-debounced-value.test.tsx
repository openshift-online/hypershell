import { act, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { useDebouncedValue } from "./use-debounced-value";

describe("useDebouncedValue", () => {
  afterEach(() => {
    vi.useRealTimers();
  });

  it("publishes only the latest value after the complete delay", async () => {
    vi.useFakeTimers();
    const { rerender, result } = renderHook(
      ({ value }) => useDebouncedValue(value, 250),
      { initialProps: { value: "initial" } },
    );

    rerender({ value: "east" });
    await act(() => vi.advanceTimersByTimeAsync(200));
    rerender({ value: "west" });
    await act(() => vi.advanceTimersByTimeAsync(249));
    expect(result.current).toBe("initial");

    await act(() => vi.advanceTimersByTimeAsync(1));
    expect(result.current).toBe("west");
  });

  it("clears its pending timer when unmounted", () => {
    vi.useFakeTimers();
    const { rerender, unmount } = renderHook(
      ({ value }) => useDebouncedValue(value, 250),
      { initialProps: { value: "initial" } },
    );

    rerender({ value: "next" });
    expect(vi.getTimerCount()).toBe(1);
    unmount();
    expect(vi.getTimerCount()).toBe(0);
  });
});
