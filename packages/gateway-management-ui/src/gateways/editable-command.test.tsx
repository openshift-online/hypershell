import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { IntlProvider } from "react-intl";
import { describe, expect, it } from "vitest";

import { EditableCommand, sanitizeFieldValue } from "./editable-command";

const marker = "ZMARK";

// A command template with the same marker used twice, mirroring the real setup
// script where the provider name appears in both the create and inference steps.
function Harness() {
  const [value, setValue] = useState("alpha");
  return (
    <IntlProvider locale="en">
      <EditableCommand
        copyAriaLabel="Copy the command"
        copyText={`run --a ${value} --b ${value}`}
        labels={{ [marker]: "Value (editable)" }}
        markers={[marker]}
        onFieldChange={(_fieldMarker, next) => {
          setValue(next);
        }}
        templateCommand={`run --a ${marker} --b ${marker}`}
        values={{ [marker]: value }}
      />
    </IntlProvider>
  );
}

async function fields(): Promise<HTMLElement[]> {
  return waitFor(() => {
    const found = screen.getAllByRole("textbox", { name: "Value (editable)" });
    expect(found).toHaveLength(2);
    return found;
  });
}

describe("sanitizeFieldValue", () => {
  it("keeps shell-safe identifier characters", () => {
    expect(sanitizeFieldValue("claude-haiku-4-5")).toBe("claude-haiku-4-5");
    expect(sanitizeFieldValue("my-gcp_1.0")).toBe("my-gcp_1.0");
  });

  it("strips whitespace and shell metacharacters", () => {
    expect(sanitizeFieldValue("a b")).toBe("ab");
    expect(sanitizeFieldValue("na$(me)")).toBe("name");
    expect(sanitizeFieldValue("x\ny")).toBe("xy");
  });
});

describe("EditableCommand", () => {
  it("renders the marked value as inline editable fields", async () => {
    render(<Harness />);

    const [first, second] = await fields();
    expect(first?.textContent).toBe("alpha");
    expect(second?.textContent).toBe("alpha");
  });

  it("mirrors an edit across every slot sharing the marker", async () => {
    render(<Harness />);

    const [first, second] = await fields();
    if (first) {
      first.textContent = "beta";
      fireEvent.input(first);
    }

    await waitFor(() => {
      expect(second?.textContent).toBe("beta");
    });
    expect(first?.textContent).toBe("beta");
  });

  it("rejects characters that are not shell-safe as they are typed", async () => {
    render(<Harness />);

    const [first] = await fields();
    if (first) {
      first.textContent = "b c";
      fireEvent.input(first);
    }

    expect(first?.textContent).toBe("bc");
  });

  it("copies the resolved command with the edited value in place", async () => {
    const user = userEvent.setup();
    render(<Harness />);

    const [first] = await fields();
    if (first) {
      first.textContent = "prod";
      fireEvent.input(first);
    }

    await user.click(screen.getByRole("button", { name: "Copy the command" }));

    expect(await navigator.clipboard.readText()).toBe("run --a prod --b prod");
  });
});
