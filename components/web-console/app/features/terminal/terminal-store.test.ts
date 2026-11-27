import { describe, expect, it, vi } from "vitest";

import {
  createFakeTerminal,
  type FakeRuntimeScript,
} from "../../application/terminal/testing/fake-terminal";
import { createTerminalStore } from "./terminal-store";

function setup(script: FakeRuntimeScript = {}) {
  const terminal = createFakeTerminal(script);
  const onGatewayStateMayHaveChanged = vi.fn();
  const store = createTerminalStore({
    onGatewayStateMayHaveChanged,
    operations: terminal.operations,
  });
  store.setSession({ identity: "ada", status: "signed-in" });
  return { onGatewayStateMayHaveChanged, store, terminal };
}

async function settle() {
  for (let index = 0; index < 5; index += 1) {
    await Promise.resolve();
  }
}

describe("terminal store", () => {
  // Verifies: WEB-TERM-12
  it("clears the completion announcement when the next command starts", async () => {
    const { store } = setup();
    store.open();
    await settle();

    store.submit("version");
    await settle();
    const first = store.getSnapshot().announcement;
    expect(first).toMatchObject({ exitCode: 0, kind: "finished" });

    store.submit("version");
    expect(store.getSnapshot().announcement).toBeNull();
    await settle();
    const second = store.getSnapshot().announcement;
    expect(second).toMatchObject({ exitCode: 0, kind: "finished" });
    expect(second?.serial).not.toBe(first?.serial);
  });

  // Verifies: WEB-TERM-08, WEB-TERM-07
  it("scrolls to an entry only for an explicit open and forgets it on close", () => {
    const { store } = setup();
    const entry = store.startRunbookEntry("hsctl create gateway --name a");

    store.open({ entryId: entry.entryId });
    expect(store.getSnapshot().scrollRequest).toMatchObject({
      entryId: entry.entryId,
    });
    store.open();
    expect(store.getSnapshot().scrollRequest).toBeNull();

    store.open({ entryId: entry.entryId });
    store.close();
    expect(store.getSnapshot().scrollRequest).toBeNull();
    store.open();
    expect(store.getSnapshot().scrollRequest).toBeNull();
  });

  // Verifies: WEB-TERM-09, WEB-TERM-05.01
  it("reloads the runtime with progress after a cancel and offers Retry when that fails", async () => {
    const script: FakeRuntimeScript = { respond: () => "forever" };
    const { onGatewayStateMayHaveChanged, store, terminal } = setup(script);
    store.open();
    await settle();
    expect(store.getSnapshot().runtime).toEqual({ kind: "ready" });

    script.loadFailure = "crashed";
    store.submit("list gateways");
    store.cancelRunning();
    await settle();

    expect(terminal.runtime.loadCount).toBe(2);
    expect(store.getSnapshot().runtime).toEqual({
      failureKind: "load-failed",
      kind: "failed",
    });
    expect(store.getSnapshot().running).toBeNull();
    expect(onGatewayStateMayHaveChanged).toHaveBeenCalledTimes(1);

    delete script.loadFailure;
    store.retryLoad();
    expect(store.getSnapshot().runtime).toEqual({
      kind: "loading",
      progress: null,
    });
    await settle();
    expect(store.getSnapshot().runtime).toEqual({ kind: "ready" });
  });

  // Verifies: WEB-TERM-09
  it("waits for the next open to reload when the drawer is closed", async () => {
    const { store, terminal } = setup({ respond: () => "forever" });
    store.open();
    await settle();
    store.submit("list gateways");
    store.close();
    store.cancelRunning();
    await settle();

    expect(store.getSnapshot().runtime).toEqual({ kind: "idle" });
    expect(terminal.runtime.loadCount).toBe(1);
    store.open();
    await settle();
    expect(terminal.runtime.loadCount).toBe(2);
  });
});
