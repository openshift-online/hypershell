import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { useE2eDiagnostics } from "./hooks";

const { requestDiagnostics } = vi.hoisted(() => ({
  requestDiagnostics: vi.fn(),
}));

vi.mock("./api-context", () => ({
  useFleetApi: () => ({ getE2eDiagnostics: requestDiagnostics }),
}));

function setup() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  function Wrapper({ children }: { children: ReactNode }) {
    return (
      <QueryClientProvider client={client}>{children}</QueryClientProvider>
    );
  }
  return { client, wrapper: Wrapper };
}

describe("useE2eDiagnostics", () => {
  beforeEach(() => {
    vi.useFakeTimers();
    requestDiagnostics.mockReset().mockResolvedValue({
      data: { diagnostics: [] },
      generatedAt: "2026-01-01T00:00:00Z",
      stale: false,
      error: null,
    });
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it("fetches and polls only while expanded", async () => {
    const { wrapper, client } = setup();
    const { rerender, unmount } = renderHook(
      ({ expanded }) => useE2eDiagnostics("tenant/ns/run", expanded),
      { wrapper, initialProps: { expanded: false } },
    );
    expect(requestDiagnostics).not.toHaveBeenCalled();
    await act(async () => {
      rerender({ expanded: true });
      await vi.advanceTimersByTimeAsync(0);
    });
    expect(requestDiagnostics).toHaveBeenCalledTimes(1);
    await act(async () => {
      await vi.advanceTimersByTimeAsync(30_000);
    });
    expect(requestDiagnostics).toHaveBeenCalledTimes(2);
    rerender({ expanded: false });
    await act(async () => {
      await vi.advanceTimersByTimeAsync(60_000);
    });
    expect(requestDiagnostics).toHaveBeenCalledTimes(2);
    unmount();
    client.clear();
  });

  it("reuses fresh diagnostics on reopening and remounting", async () => {
    const { wrapper, client } = setup();
    const { rerender, unmount } = renderHook(
      ({ expanded }) => useE2eDiagnostics("tenant/ns/run", expanded),
      { wrapper, initialProps: { expanded: true } },
    );
    await act(async () => {
      await vi.advanceTimersByTimeAsync(1);
    });
    expect(requestDiagnostics).toHaveBeenCalledTimes(1);
    rerender({ expanded: false });
    await act(async () => {
      await vi.advanceTimersByTimeAsync(1000);
    });
    rerender({ expanded: true });
    expect(requestDiagnostics).toHaveBeenCalledTimes(1);
    unmount();
    const remounted = renderHook(
      () => useE2eDiagnostics("tenant/ns/run", true),
      { wrapper },
    );
    expect(requestDiagnostics).toHaveBeenCalledTimes(1);
    remounted.unmount();
    client.clear();
  });

  it("aborts an obsolete run request and isolates composition-owned caches", () => {
    const signals: AbortSignal[] = [];
    requestDiagnostics.mockImplementation(
      (_id: string, signal: AbortSignal) => {
        signals.push(signal);
        return new Promise(() => undefined);
      },
    );
    const first = setup();
    const second = setup();
    const firstView = renderHook(({ run }) => useE2eDiagnostics(run, true), {
      wrapper: first.wrapper,
      initialProps: { run: "tenant/ns/run-1" },
    });
    firstView.rerender({ run: "tenant/ns/run-2" });
    expect(signals[0]?.aborted).toBe(true);
    const secondView = renderHook(
      () => useE2eDiagnostics("tenant/ns/run-2", true),
      { wrapper: second.wrapper },
    );
    expect(requestDiagnostics).toHaveBeenCalledTimes(3);
    firstView.unmount();
    secondView.unmount();
    expect(signals.every((signal) => signal.aborted)).toBe(true);
    first.client.clear();
    second.client.clear();
  });
});
