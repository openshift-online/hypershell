import { createTerminalOperations } from "../application/terminal/terminal-operations";
import { createBrowserTerminalPreferences } from "../adapters/terminal/browser-terminal-preferences";
import { createTerminalSessionCheck } from "../adapters/terminal/terminal-session-check";
import { createWorkerHsctlRuntime } from "../adapters/terminal/worker-hsctl-runtime";
import { createTerminalObservability } from "../adapters/observability/terminal-observability";
import { gatewayObservability } from "./gateway-composition";
import { sessionGateway } from "./session-composition";

/*
 * Browser-lifetime wiring for the hsctl terminal (UIS-HEX-07). Creating these
 * adapters requests nothing: the worker, `wasm_exec.js`, and `hsctl.wasm` are
 * fetched only when a signed-in user first opens the drawer (WEB-TERM-09).
 * Clock and identifiers come from the shared gateway workflow runtime and are
 * read lazily, at invocation time.
 */

export const terminalObservability = createTerminalObservability();

export const terminalOperations = createTerminalOperations({
  probes: terminalObservability.probes,
  runtime: createWorkerHsctlRuntime(),
  runtimeContext: {
    createCorrelationId: () =>
      gatewayObservability.runtime.createCorrelationId(),
    createTraceId: () => gatewayObservability.runtime.createTraceId(),
    elapsedMilliseconds: () => globalThis.performance.now(),
    now: () => gatewayObservability.runtime.now(),
  },
  session: createTerminalSessionCheck(sessionGateway),
});

export const terminalPreferences = createBrowserTerminalPreferences();
