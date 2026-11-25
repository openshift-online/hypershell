import { describe, expect, it } from "vitest";

import { createFakeTerminal } from "./testing/fake-terminal";
import { terminalProbeCatalog } from "./terminal-probes";

describe("terminal operations", () => {
  // Verifies: WEB-TERM-13.01
  it("publishes started and finished facts with only the command path", async () => {
    const terminal = createFakeTerminal({
      respond: () => ({
        exitCode: 0,
        mutated: true,
        stderr: "",
        stdout: "deleted 3f2a",
      }),
    });

    const outcome = await terminal.operations.runCommand([
      "delete",
      "gateway",
      "3f2a",
      "--yes",
    ]);

    expect(outcome).toMatchObject({ kind: "finished", sessionExpired: false });
    expect(terminal.probes.map((probe) => probe.name)).toEqual([
      "terminal.command.started",
      "terminal.command.finished",
    ]);
    expect(terminal.probes[1]?.fields).toEqual({
      commandPath: "delete gateway",
      durationMs: 5,
      exitStatusClass: "zero",
      mutated: true,
      sessionExpired: false,
    });
    const serialized = JSON.stringify(terminal.probes);
    expect(serialized).not.toContain("3f2a");
    expect(serialized).not.toContain("--yes");
    expect(Object.isFrozen(terminal.probes[1])).toBe(true);
  });

  // Verifies: WEB-TERM-10.02
  it("checks the session after a failing command", async () => {
    const terminal = createFakeTerminal({
      respond: () => ({
        exitCode: 1,
        mutated: false,
        stderr: "console session expired",
        stdout: "",
      }),
    });
    terminal.sessionExpired.value = true;

    const outcome = await terminal.operations.runCommand(["list", "gateways"]);

    expect(outcome).toMatchObject({ kind: "finished", sessionExpired: true });
    expect(terminal.probes[1]?.fields).toMatchObject({
      exitStatusClass: "non-zero",
      sessionExpired: true,
    });
  });

  // Verifies: WEB-TERM-05.01
  it("reports a cancelled command once", async () => {
    const terminal = createFakeTerminal({ respond: () => "forever" });
    const running = terminal.operations.runCommand(["list", "gateways"]);
    terminal.operations.cancelCommand();

    await expect(running).resolves.toEqual({ kind: "cancelled" });
    expect(
      terminal.probes.filter(
        (probe) => probe.name === "terminal.command.finished",
      ),
    ).toHaveLength(1);
    expect(terminal.probes.at(-1)?.fields).toMatchObject({
      exitStatusClass: "cancelled",
    });
  });

  // Verifies: WEB-TERM-09
  it("reports runtime load success and failure with a duration", async () => {
    const ok = createFakeTerminal();
    await expect(ok.operations.loadRuntime(() => undefined)).resolves.toEqual({
      outcome: "succeeded",
    });
    expect(ok.probes.map((probe) => probe.name)).toEqual([
      "terminal.runtime-load.started",
      "terminal.runtime-load.completed",
    ]);

    const missing = createFakeTerminal({ loadFailure: "unavailable" });
    await expect(
      missing.operations.loadRuntime(() => undefined),
    ).resolves.toEqual({ failureKind: "unavailable", outcome: "failed" });
    const broken = createFakeTerminal({ loadFailure: "crashed" });
    await expect(
      broken.operations.loadRuntime(() => undefined),
    ).resolves.toEqual({ failureKind: "load-failed", outcome: "failed" });
    expect(broken.probes.at(-1)?.fields).toEqual({
      durationMs: 5,
      failureKind: "load-failed",
      outcome: "failed",
    });
  });

  // Verifies: WEB-TERM-13
  it("publishes drawer and refusal facts and catalogs every probe", () => {
    const terminal = createFakeTerminal();
    terminal.operations.drawerOpened(false);
    terminal.operations.drawerClosed();
    terminal.operations.refuseCommand("shell-operator");

    expect(terminal.probes.map((probe) => [probe.name, probe.fields])).toEqual([
      ["terminal.drawer.opened", { signedIn: false }],
      ["terminal.drawer.closed", {}],
      ["terminal.command.refused", { reason: "shell-operator" }],
    ]);
    expect(terminalProbeCatalog.map((entry) => entry.name).sort()).toEqual([
      "terminal.command.finished",
      "terminal.command.refused",
      "terminal.command.started",
      "terminal.drawer.closed",
      "terminal.drawer.opened",
      "terminal.runtime-load.completed",
      "terminal.runtime-load.started",
    ]);
  });
});
