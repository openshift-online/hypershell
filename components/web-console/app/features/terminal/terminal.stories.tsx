import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  Drawer,
  DrawerContent,
  DrawerPanelContent,
  PageSection,
} from "@patternfly/react-core";
import { useRef, useState, type CSSProperties, type ReactNode } from "react";
import { MemoryRouter } from "react-router";

import {
  appendTerminalNotice,
  appendTerminalOutput,
  emptyTerminalHistory,
  splitOutputLines,
  startTerminalEntry,
  type TerminalHistory,
} from "../../application/terminal/terminal-history";
import type { TerminalDock } from "../../application/terminal/terminal-layout";
import {
  createFakeTerminal,
  createMemoryTerminalPreferences,
} from "../../application/terminal/testing/fake-terminal";
import { TerminalProvider } from "./terminal-context";
import { TerminalDrawer } from "./terminal-drawer";
import { TerminalPanel, type TerminalPanelProps } from "./terminal-panel";
import type { TerminalRuntimeStatus } from "./terminal-store";
import { TerminalTrigger } from "./terminal-trigger";

/*
 * Terminal states for review (WEB-TERM-14). Stories render the presentational
 * panel with fixture history, or the composed drawer with a scripted fake
 * runtime; no story loads a worker or the hsctl WebAssembly module.
 */

const pageText =
  "Console page content reflows beside or above the terminal instead of being covered.";
const signInHref = "/auth/login?return_to=%2Fgateways";
const regionLabel = "hsctl terminal";

function entry(
  history: TerminalHistory,
  command: string,
  stdout: string,
  options: { exitCode?: number; runbook?: boolean; stderr?: string } = {},
): TerminalHistory {
  const started = startTerminalEntry(
    history,
    options.runbook === true ? "runbook" : "typed",
    command,
  );
  let next = appendTerminalOutput(
    started.history,
    started.entryId,
    splitOutputLines(stdout),
    "stdout",
  );
  if (options.stderr !== undefined) {
    next = appendTerminalOutput(
      next,
      started.entryId,
      splitOutputLines(options.stderr),
      "stderr",
    );
  }
  if (options.exitCode !== undefined && options.exitCode !== 0) {
    next = appendTerminalNotice(next, started.entryId, {
      exitCode: options.exitCode,
      kind: "exit-code",
    });
  }
  return next;
}

const idleHistory = entry(
  entry(
    emptyTerminalHistory,
    "hsctl list gateways",
    "ID          NAME          STATUS\n3f2a9c1e    team-prod     Ready\n7b1d0e22    team-staging  Provisioning\n",
  ),
  "hsctl create gateway --name docs-demo",
  "Created gateway docs-demo (9c4e2f10)\n",
  { runbook: true },
);

const runningHistory = startTerminalEntry(
  idleHistory,
  "typed",
  "hsctl get gateway 3f2a9c1e",
).history;

const failedHistory = entry(idleHistory, "hsctl delete gateway 3f2a9c1e", "", {
  exitCode: 1,
  stderr:
    "Error: deleting a gateway needs confirmation; run again with --yes\n",
});

const longHistory = entry(
  emptyTerminalHistory,
  "hsctl list gateways --size 200",
  Array.from(
    { length: 200 },
    (_value, index) =>
      `${String(index).padStart(4, "0")}aa  gateway-${String(index)}  Ready  https://gateway-${String(index)}.apps.example.test`,
  ).join("\n"),
);

function noop() {
  return undefined;
}

function PanelFrame({
  dock,
  ...panel
}: Partial<TerminalPanelProps> & {
  dock: TerminalDock;
  runtime: TerminalRuntimeStatus;
}) {
  const initialFocusRef = useRef<HTMLElement | null>(null);
  const [input, setInput] = useState(panel.input ?? "");
  const size = dock === "bottom" ? 320 : 480;
  const props: TerminalPanelProps = {
    announcement: null,
    bounds: dock === "bottom" ? { max: 560, min: 180 } : { max: 832, min: 320 },
    canDockRight: true,
    commandHistory: [],
    controlsId: "terminal-story-panel",
    dock,
    history: emptyTerminalHistory,
    initialFocusRef,
    input,
    isRunning: false,
    onCancel: noop,
    onClear: noop,
    onClose: noop,
    onDockChange: noop,
    onInputChange: setInput,
    onInputFocusChange: noop,
    onInterrupt: noop,
    onResize: noop,
    onRetry: noop,
    onSubmit: () => true,
    scrollRequest: null,
    sessionStatus: "signed-in",
    signInHref,
    size,
    titleId: "terminal-story-title",
    ...panel,
  };
  return (
    <div style={{ blockSize: "640px" }}>
      <Drawer
        isExpanded
        isInline
        position={dock === "right" ? "end" : "bottom"}
      >
        <DrawerContent
          panelContent={
            <DrawerPanelContent
              aria-label={regionLabel}
              id="terminal-story-panel"
              role="region"
              style={
                {
                  "--pf-v6-c-drawer__panel--FlexBasis": `${String(size)}px`,
                } as CSSProperties
              }
            >
              <TerminalPanel {...props} />
            </DrawerPanelContent>
          }
        >
          <PageSection>{pageText}</PageSection>
        </DrawerContent>
      </Drawer>
    </div>
  );
}

function ComposedTerminal({ children }: { children?: ReactNode }) {
  const [terminal] = useState(() =>
    createFakeTerminal({
      respond: () => ({
        exitCode: 0,
        mutated: false,
        stderr: "",
        stdout: "hsctl version v0.0.0-story\n",
      }),
    }),
  );
  const [preferences] = useState(() => createMemoryTerminalPreferences());
  return (
    <MemoryRouter initialEntries={["/gateways"]}>
      <TerminalProvider
        operations={terminal.operations}
        preferences={preferences}
        session={{ identity: "story-user", status: "signed-in" }}
      >
        <div style={{ blockSize: "640px" }}>
          <TerminalDrawer>
            <PageSection>
              <TerminalTrigger />
              {pageText}
              {children}
            </PageSection>
          </TerminalDrawer>
        </div>
      </TerminalProvider>
    </MemoryRouter>
  );
}

const meta = {
  title: "Production/Terminal/hsctl terminal",
  component: PanelFrame,
  parameters: { layout: "fullscreen" },
  args: { dock: "bottom", runtime: { kind: "ready" } },
} satisfies Meta<typeof PanelFrame>;

export default meta;
type Story = StoryObj<typeof meta>;

/** Closed: only the masthead trigger renders; open it to run `version`. */
export const Closed: Story = {
  render: () => <ComposedTerminal />,
};

export const SignedOut: Story = {
  args: { sessionStatus: "signed-out" },
};

export const Loading: Story = {
  args: {
    runtime: {
      kind: "loading",
      progress: { loadedBytes: 3_100_000, totalBytes: 7_800_000 },
    },
  },
};

export const LoadFailed: Story = {
  args: { runtime: { failureKind: "load-failed", kind: "failed" } },
};

export const IdleBottomDock: Story = {
  args: { history: idleHistory },
};

export const IdleRightDock: Story = {
  args: { dock: "right", history: idleHistory },
};

export const RunningCommand: Story = {
  args: { history: runningHistory, isRunning: true },
};

export const FailedCommand: Story = {
  args: {
    announcement: { exitCode: 1, kind: "finished", serial: 1 },
    history: failedHistory,
  },
};

export const LongOutput: Story = {
  args: { history: longHistory },
};
