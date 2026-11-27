import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { IntlProvider } from "react-intl";
import { describe, expect, it, vi } from "vitest";

import { englishMessages } from "../../i18n/catalog";
import type { PlatformPolicyEvaluation } from "../../application/platform-policies/platform-policy-types";
import type { CedarCodeEditorProps } from "./cedar-code-editor";
import {
  allowedEvaluation,
  defaultsForbiddingEvaluation,
  editorFormValues,
  editorTestValues,
  invalidCedar,
  invalidEvaluation,
  validCedar,
  validEvaluation,
} from "./fixtures/platform-policy-editor-fixtures";
import {
  PlatformPolicyFormPage,
  type PlatformPolicyFormPageProps,
} from "./platform-policy-form-page";
import {
  emptyPlatformPolicyFormValues,
  emptyPlatformPolicyTestValues,
} from "./platform-policy-form-model";

const markersLabel = "editor markers";
const markerText = (diagnostic: { line: number; message: string }) =>
  `marker ${String(diagnostic.line)}: ${diagnostic.message}`;

// Monaco cannot run in jsdom; the wrapper is replaced by a plain text area
// that exposes the same props, including the line diagnostics.
vi.mock("./cedar-code-editor", () => ({
  CedarCodeEditor: ({
    ariaLabel,
    code,
    diagnostics,
    onChange,
  }: CedarCodeEditorProps) => (
    <div>
      <textarea
        aria-label={ariaLabel}
        onChange={(event) => {
          onChange(event.target.value);
        }}
        value={code}
      />
      <ul aria-label={markersLabel}>
        {diagnostics.map((diagnostic) => (
          <li key={diagnostic.message}>{markerText(diagnostic)}</li>
        ))}
      </ul>
    </div>
  ),
}));

function Harness({
  cedar,
  evaluation,
  initial = {},
  onSave = vi.fn(),
  overrides = {},
}: {
  cedar: string;
  evaluation?: PlatformPolicyEvaluation;
  initial?: Partial<ReturnType<typeof editorFormValues>>;
  onSave?: () => void;
  overrides?: Partial<PlatformPolicyFormPageProps>;
}) {
  const [values, setValues] = useState(() => ({
    ...editorFormValues(cedar),
    ...initial,
  }));
  const [testValues, setTestValues] = useState(editorTestValues);
  return (
    <IntlProvider locale="en" messages={englishMessages}>
      <PlatformPolicyFormPage
        cancelHref="/admin/platform-policies"
        evaluation={evaluation}
        evaluationFailed={false}
        isEditing={false}
        isEvaluating={false}
        isSaving={false}
        onCancel={vi.fn()}
        onSave={onSave}
        onTestValuesChange={setTestValues}
        onValuesChange={setValues}
        schemaVersion="v1"
        testValues={testValues}
        values={values}
        {...overrides}
      />
    </IntlProvider>
  );
}

const saveButton = () => screen.getByRole("button", { name: "Create policy" });

describe("PlatformPolicyFormPage", () => {
  // Verifies: WEB-PADM-05
  it("shows diagnostics at the offending line and blocks saving", async () => {
    const onSave = vi.fn();
    render(
      <Harness
        cedar={invalidCedar}
        evaluation={invalidEvaluation}
        onSave={onSave}
      />,
    );
    expect(screen.getByText("2 problems", { selector: "span" })).toBeTruthy();
    const markers = screen.getByRole("list", { name: "editor markers" });
    expect(within(markers).getByText(/^marker 7: Line 7:/u)).toBeTruthy();
    expect(within(markers).getByText(/^marker 9: Line 9:/u)).toBeTruthy();
    expect(
      screen.getByText(
        "Line 7: context.cpuRequest is not available. Suggested fix: use context.cpuRequestMillis (Long, millicores); 4 CPU is 4000.",
      ),
    ).toBeTruthy();
    expect(screen.getByText("Fix 2 problems before you save.")).toBeTruthy();
    expect(saveButton().getAttribute("aria-disabled")).toBe("true");
    await userEvent.click(saveButton());
    expect(onSave).not.toHaveBeenCalled();
    expect(
      screen.getByText("Fix the problems in the rules to see the preview."),
    ).toBeTruthy();
  });

  // Verifies: WEB-PADM-05
  it("reports a valid draft and saves it", async () => {
    const onSave = vi.fn();
    render(
      <Harness
        cedar={validCedar}
        evaluation={validEvaluation}
        onSave={onSave}
      />,
    );
    expect(screen.getByText("Valid, 3 statements")).toBeTruthy();
    expect(
      screen.getByText(
        "Valid against the HyperShell Cedar schema v1. All statements use forbid.",
      ),
    ).toBeTruthy();
    expect(
      screen.getByText(
        "The fleet defaults pass: CPU request 500m, memory request 1Gi, GPUs 0.",
      ),
    ).toBeTruthy();
    await userEvent.click(saveButton());
    expect(onSave).toHaveBeenCalledTimes(1);
  });

  // Verifies: WEB-PADM-06
  it("inserts a CPU request range rule from the snippet form", async () => {
    render(<Harness cedar="" evaluation={undefined} />);
    expect(
      screen.getByText(
        "Write at least one forbid statement, or use Add a rule to start.",
      ),
    ).toBeTruthy();
    await userEvent.click(screen.getByRole("button", { name: "Add a rule" }));
    await userEvent.click(
      screen.getByRole("menuitem", { name: /CPU request range/u }),
    );
    const dialog = screen.getByRole("dialog");
    expect(
      within(dialog).getByText("Add rule: CPU request range"),
    ).toBeTruthy();
    expect(within(dialog).getByText("CreateSandbox")).toBeTruthy();
    await userEvent.type(within(dialog).getByLabelText("Minimum"), "abc");
    expect(
      within(dialog).getByText("Enter a CPU quantity such as 250m or 4."),
    ).toBeTruthy();
    await userEvent.clear(within(dialog).getByLabelText("Minimum"));
    await userEvent.type(within(dialog).getByLabelText("Minimum"), "250m");
    await userEvent.type(within(dialog).getByLabelText("Maximum"), "4");
    await userEvent.click(
      within(dialog).getByRole("button", { name: "Insert rule" }),
    );
    const editor = screen.getByRole<HTMLTextAreaElement>("textbox", {
      name: "Cedar policy source",
    });
    expect(editor.value).toContain(
      "context.cpuRequestMillis < 250 || context.cpuRequestMillis > 4000",
    );
    expect(editor.value).toContain(
      'action == OpenShell::Action::"CreateSandbox"',
    );
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  // Verifies: WEB-PADM-06
  it("opens snippet forms from the Snippets tab and validates them", async () => {
    render(
      <Harness
        cedar={validCedar}
        evaluation={validEvaluation}
        overrides={{ initialPanel: "snippets" }}
      />,
    );
    await userEvent.click(
      screen.getByRole("button", { name: "Add Allowed image registries" }),
    );
    const dialog = screen.getByRole("dialog");
    await userEvent.click(
      within(dialog).getByRole("button", { name: "Insert rule" }),
    );
    expect(within(dialog).getByText("Enter a value.")).toBeTruthy();
    await userEvent.click(
      within(dialog).getByRole("button", { name: "Cancel" }),
    );
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  // Verifies: WEB-PADM-07
  it("lists what the language offers and what is not available", () => {
    render(<Harness cedar={validCedar} evaluation={validEvaluation} />);
    const table = screen.getByRole("grid", { name: "Context attributes" });
    expect(within(table).getByText("cpuRequestMillis")).toBeTruthy();
    expect(within(table).getAllByText("Long, millicores").length).toBe(2);
    expect(within(table).getByText("500m = 500")).toBeTruthy();
    expect(within(table).getByText("1Gi = 1073741824")).toBeTruthy();
    expect(within(table).getByText("String, image reference")).toBeTruthy();
    expect(screen.getByText("like")).toBeTruthy();
    expect(screen.getByText(".containsAny()")).toBeTruthy();
    expect(screen.getByText("CreateSandbox")).toBeTruthy();
    expect(screen.getByText("ResumeSandbox (later)")).toBeTruthy();
    expect(screen.getByText("permit")).toBeTruthy();
    expect(screen.getByText("ip()")).toBeTruthy();
    expect(screen.getByText("Quotas over other sandboxes")).toBeTruthy();
    expect(screen.getByText("Time or date conditions")).toBeTruthy();
    expect(screen.getByText("String (gateway name)")).toBeTruthy();
  });

  // Verifies: WEB-PADM-08
  it("shows the statements that reject the test request", async () => {
    render(
      <Harness
        cedar={validCedar}
        evaluation={validEvaluation}
        overrides={{ initialPanel: "test" }}
      />,
    );
    expect(screen.getByText("Rejected by 2 statements")).toBeTruthy();
    expect(
      screen.getByText(
        "Line 2: CPU request: at least 250m and CPU request: at most 4",
      ),
    ).toBeTruthy();
    expect(screen.getByText(/^Line 17: forbid/u)).toBeTruthy();
    expect(
      screen.getByRole<HTMLInputElement>("textbox", { name: "CPU request" })
        .value,
    ).toBe("100m");
    expect(
      screen
        .getByRole("textbox", { name: "Memory request" })
        .getAttribute("placeholder"),
    ).toBe("1Gi (default)");
    await userEvent.type(
      screen.getByRole("textbox", { name: "CPU limit" }),
      "x",
    );
    expect(
      screen.getByText("Enter a CPU quantity such as 250m or 4."),
    ).toBeTruthy();
  });

  // Verifies: WEB-PADM-08
  it("shows Allowed when no statement rejects the test request", () => {
    render(
      <Harness
        cedar={validCedar}
        evaluation={allowedEvaluation}
        overrides={{ initialPanel: "test" }}
      />,
    );
    expect(screen.getByText("Allowed")).toBeTruthy();
    expect(screen.getByText(/The test request is allowed\./u)).toBeTruthy();
  });

  // Verifies: WEB-PADM-09.01
  it("previews rules and denial text and blocks a defaults-forbidding draft", async () => {
    const { unmount } = render(
      <Harness cedar={validCedar} evaluation={validEvaluation} />,
    );
    expect(screen.getByText("On the gateway Policies tab")).toBeTruthy();
    expect(screen.getByText("250m to 4")).toBeTruthy();
    expect(screen.getByText("Up to 16Gi")).toBeTruthy();
    expect(screen.getAllByText("Shown as Cedar").length).toBeGreaterThan(0);
    expect(
      screen.getByText(/Rejected by 1 policy:/u, { selector: "code, pre, *" }),
    ).toBeTruthy();
    unmount();

    const onSave = vi.fn();
    render(
      <Harness
        cedar={validCedar}
        evaluation={defaultsForbiddingEvaluation}
        onSave={onSave}
      />,
    );
    expect(
      screen.getByText(/These rules reject the fleet defaults/u),
    ).toBeTruthy();
    expect(
      screen.getByText(
        "Change the rules so they allow the fleet defaults before you save.",
      ),
    ).toBeTruthy();
    await userEvent.click(saveButton());
    expect(onSave).not.toHaveBeenCalled();
  });

  // Verifies: WEB-PADM-13.01
  it("fills an empty documentation URL with the console help page on request", async () => {
    const user = userEvent.setup();
    render(
      <Harness
        cedar={validCedar}
        evaluation={validEvaluation}
        initial={{ documentationUrl: "", name: "sandbox-cpu-limit" }}
      />,
    );

    await user.click(
      screen.getByRole("button", {
        name: "Use the console resource limit help page",
      }),
    );

    expect(
      screen.getByLabelText<HTMLInputElement>("Documentation URL").value,
    ).toBe(
      `${globalThis.location.origin}/docs/refusals/resource-limits?policy=sandbox-cpu-limit`,
    );
    expect(
      screen.queryByRole("button", {
        name: "Use the console resource limit help page",
      }),
    ).toBeNull();
  });

  // Verifies: WEB-PADM-13.02
  it("does not offer to replace an entered documentation URL", () => {
    render(
      <Harness
        cedar={validCedar}
        evaluation={validEvaluation}
        initial={{ documentationUrl: "https://example.com/limits" }}
      />,
    );

    expect(
      screen.queryByRole("button", {
        name: "Use the console resource limit help page",
      }),
    ).toBeNull();
    expect(
      screen.getByLabelText<HTMLInputElement>("Documentation URL").value,
    ).toBe("https://example.com/limits");
  });

  // Verifies: WEB-PADM-09.02
  it("warns against secrets in the message for users", () => {
    render(<Harness cedar={validCedar} evaluation={validEvaluation} />);
    expect(
      screen.getByText(
        "Everyone who uses an affected gateway can read this message. Do not include secrets, credentials, or internal hostnames.",
      ),
    ).toBeTruthy();
  });

  // Verifies: WEB-PADM-01.02, WEB-PADM-10
  it("presents denied, conflicted, invalid, and failed saves", () => {
    const { rerender } = render(
      <Harness
        cedar={validCedar}
        evaluation={validEvaluation}
        overrides={{ saveFailure: { kind: "denied" } }}
      />,
    );
    expect(
      screen.getByText(
        "You do not have permission to change platform policies",
      ),
    ).toBeTruthy();
    expect(screen.queryByRole("button", { name: /try again/iu })).toBeNull();
    rerender(
      <Harness
        cedar={validCedar}
        evaluation={validEvaluation}
        overrides={{ saveFailure: { kind: "conflicted" } }}
      />,
    );
    expect(
      screen.getByText(
        "A policy named sandbox-resource-baseline already exists",
      ),
    ).toBeTruthy();
    rerender(
      <Harness
        cedar={validCedar}
        evaluation={validEvaluation}
        overrides={{ saveFailure: { kind: "invalid", reason: "bad cedar" } }}
      />,
    );
    expect(screen.getByText("bad cedar")).toBeTruthy();
    rerender(
      <Harness
        cedar={validCedar}
        evaluation={validEvaluation}
        overrides={{ saveFailure: { kind: "unavailable" } }}
      />,
    );
    expect(screen.getByText("Could not save the policy")).toBeTruthy();
  });

  it("requires details and manages gateway targets", async () => {
    function Empty() {
      const [values, setValues] = useState({
        ...emptyPlatformPolicyFormValues,
        cedar: validCedar,
      });
      return (
        <IntlProvider locale="en" messages={englishMessages}>
          <PlatformPolicyFormPage
            cancelHref="/x"
            evaluation={validEvaluation}
            evaluationFailed={false}
            isEditing={false}
            isEvaluating={false}
            isSaving={false}
            onCancel={vi.fn()}
            onSave={vi.fn()}
            onTestValuesChange={vi.fn()}
            onValuesChange={setValues}
            testValues={emptyPlatformPolicyTestValues}
            values={values}
          />
        </IntlProvider>
      );
    }
    render(<Empty />);
    await userEvent.click(saveButton());
    expect(screen.getByText("Enter a name.")).toBeTruthy();
    expect(screen.getByText("Enter a message for users.")).toBeTruthy();
    await userEvent.click(screen.getByLabelText("Selected gateways"));
    expect(screen.getByText("Add at least one gateway.")).toBeTruthy();
    const input = screen.getByRole("textbox", {
      name: "Find gateways to include",
    });
    await userEvent.type(input, "gw-1{Enter}");
    expect(screen.getByText("gw-1")).toBeTruthy();
    await userEvent.click(screen.getByRole("button", { name: "Remove gw-1" }));
    expect(screen.queryByText("gw-1")).toBeNull();
    expect(
      screen.getByText(
        "Valid against the HyperShell Cedar schema. All statements use forbid.",
      ),
    ).toBeTruthy();
  });

  it("shows checking, unavailable, and edit-mode states", () => {
    const { rerender } = render(
      <Harness
        cedar={validCedar}
        evaluation={undefined}
        overrides={{ initialPanel: "test", isEvaluating: true }}
      />,
    );
    expect(screen.getByText("Checking")).toBeTruthy();
    expect(screen.getByText("Wait for validation to finish.")).toBeTruthy();
    rerender(
      <Harness
        cedar={validCedar}
        evaluation={undefined}
        overrides={{ evaluationFailed: true }}
      />,
    );
    expect(screen.getByText("Validation unavailable")).toBeTruthy();
    rerender(
      <Harness
        cedar={validCedar}
        evaluation={validEvaluation}
        overrides={{ isEditing: true }}
      />,
    );
    expect(
      screen.getByRole("heading", { name: "Edit sandbox-resource-baseline" }),
    ).toBeTruthy();
    expect(screen.getByRole("button", { name: "Save changes" })).toBeTruthy();
    expect(
      screen.getByText(
        "The name identifies the policy in rejections and cannot change.",
      ),
    ).toBeTruthy();
  });
});
