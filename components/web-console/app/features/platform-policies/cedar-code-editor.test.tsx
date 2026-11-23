import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useEffect } from "react";
import { describe, expect, it, vi } from "vitest";

const setModelMarkers = vi.fn();

vi.mock("./cedar-monaco", () => ({
  cedarLanguageId: "cedar",
  monaco: {
    MarkerSeverity: { Error: 8 },
    editor: { setModelMarkers },
  },
}));

// The PatternFly editor needs Monaco in a real browser; this stand-in mounts
// a fake editor model so the wrapper's marker logic can be observed.
vi.mock("@patternfly/react-code-editor", () => ({
  CodeEditor: ({
    code,
    editorProps,
    onChange,
    onEditorDidMount,
    options,
  }: {
    code: string;
    editorProps: { language: string };
    onChange: (value: string) => void;
    onEditorDidMount: (editor: unknown) => void;
    options: { ariaLabel: string };
  }) => {
    useEffect(() => {
      onEditorDidMount({
        getModel: () => ({
          getLineCount: () => 3,
          getLineFirstNonWhitespaceColumn: () => 0,
          getLineMaxColumn: () => 10,
        }),
        onDidContentSizeChange: (
          listener: (event: { contentHeight: number }) => void,
        ) => {
          listener({ contentHeight: 480 });
        },
      });
    }, [onEditorDidMount]);
    return (
      <textarea
        aria-label={options.ariaLabel}
        data-language={editorProps.language}
        onChange={(event) => {
          onChange(event.target.value);
        }}
        value={code}
      />
    );
  },
}));

import { CedarCodeEditor } from "./cedar-code-editor";

describe("CedarCodeEditor", () => {
  // Verifies: WEB-PADM-05
  it("marks each diagnostic at its line in the Cedar editor", async () => {
    const onChange = vi.fn();
    render(
      <CedarCodeEditor
        ariaLabel="Cedar policy source"
        code={"forbid\n(\n)"}
        diagnostics={[
          { line: 2, message: "bad attribute" },
          { line: 9, message: "past the end" },
        ]}
        onChange={onChange}
      />,
    );
    const editor = await screen.findByRole("textbox", {
      name: "Cedar policy source",
    });
    expect(editor.getAttribute("data-language")).toBe("cedar");
    await waitFor(() => {
      expect(setModelMarkers).toHaveBeenCalled();
    });
    const markers = setModelMarkers.mock.lastCall?.[2] as {
      message: string;
      startColumn: number;
      startLineNumber: number;
    }[];
    expect(markers).toEqual([
      expect.objectContaining({
        message: "bad attribute",
        startColumn: 1,
        startLineNumber: 2,
      }),
      expect.objectContaining({ message: "past the end", startLineNumber: 3 }),
    ]);
    await userEvent.type(editor, "x");
    expect(onChange).toHaveBeenCalled();
  });
});
