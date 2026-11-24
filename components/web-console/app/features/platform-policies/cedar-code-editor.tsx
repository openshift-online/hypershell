import { CodeEditor } from "@patternfly/react-code-editor";
import { CedarCodeBlock } from "@openshift-online/hypershell-gateway-management-ui";
import { TextArea } from "@patternfly/react-core";
import { useEffect, useRef, useState } from "react";

import type * as CedarMonaco from "./cedar-monaco";
import styles from "./cedar-code-editor.module.css";

/**
 * The console's single in-browser code editor (UIS-PFLY-11), used by the
 * platform policy editor (WEB-PADM-05).
 *
 * Admission decision record (UIS-PFLY-06, UIS-PFLY-11):
 * - Gap: authors need line numbers, Cedar syntax highlighting, and inline
 *   diagnostics at the offending line. A PatternFly TextArea offers none of
 *   these, and CodeBlock is read-only.
 * - Alternatives: TextArea (no lines or markers), CodeBlock (read-only), a
 *   second editor library such as CodeMirror (duplicate implementation,
 *   rejected by UIS-PFLY-04/05). `@patternfly/react-code-editor` is the
 *   PatternFly-maintained editor and is pinned to the console's PatternFly 6
 *   release.
 * - Consumers: the platform policy create and edit pages. New consumers reuse
 *   this wrapper; no second editor implementation is allowed.
 * - API: `code`, `onChange`, `diagnostics` (line + message), `ariaLabel`,
 *   `describedBy`. The wrapper adds the Cedar language, local Monaco loading,
 *   and diagnostics-as-markers; everything else is PatternFly CodeEditor
 *   behavior.
 * - Accessibility contract: the editor has an accessible name (`ariaLabel`)
 *   and points at the helper and diagnostic list (`describedBy`). Tab inserts
 *   indentation inside Monaco, so CodeEditor's wrapper keeps keyboard escape:
 *   Shift+Tab (and Escape then Tab) moves focus out of the editor. Diagnostics
 *   are also rendered as React elements in an error list outside the editor,
 *   so screen-reader users get them without reading markers. No author text or
 *   diagnostic is ever rendered through a raw-HTML path.
 * - Bundle: Monaco is bundled locally (core editor API plus its base worker
 *   only, no language workers) and loaded with a dynamic import when the
 *   editor first mounts, so other routes do not pay for it and no CDN request
 *   is made. While it loads, the code is shown read-only so layout is stable.
 * - Retirement: replace with an upstream PatternFly editor change if
 *   PatternFly ships first-class custom-language registration.
 */

export interface CedarEditorDiagnostic {
  /** 1-based line; 0 marks a problem with no specific line. */
  line: number;
  message: string;
}

export interface CedarCodeEditorProps {
  ariaLabel: string;
  code: string;
  describedBy?: string;
  diagnostics: readonly CedarEditorDiagnostic[];
  id?: string;
  onChange: (code: string) => void;
}

type MonacoModule = typeof CedarMonaco;
type MonacoEditor = Parameters<
  NonNullable<React.ComponentProps<typeof CodeEditor>["onEditorDidMount"]>
>[0];

const markerOwner = "cedar-validation";
const editorFontSize = 14;
const editorLineHeight = 20;
const editorMinimumLines = 10;

/**
 * An explicit height sized to the draft (two spare lines, ten at minimum).
 * Monaco's measured "sizeToFit" height depends on layout timing, so a fixed
 * line-based height renders the same on every first paint.
 */
export function editorHeightPixels(code: string): number {
  const lines = Math.max(code.split("\n").length + 2, editorMinimumLines);
  return lines * editorLineHeight + 16;
}

export function CedarCodeEditor({
  ariaLabel,
  code,
  describedBy,
  diagnostics,
  id,
  onChange,
}: CedarCodeEditorProps) {
  const [monacoModule, setMonacoModule] = useState<MonacoModule>();
  const [loadFailed, setLoadFailed] = useState(false);
  const editorRef = useRef<MonacoEditor | null>(null);
  // Wrapped lines make the content taller than the line count; grow to fit so
  // no draft text is hidden behind an inner scroll on narrow screens.
  const [contentHeight, setContentHeight] = useState(0);

  useEffect(() => {
    let active = true;
    import("./cedar-monaco").then(
      (module) => {
        if (active) {
          setMonacoModule(module);
        }
      },
      () => {
        if (active) {
          setLoadFailed(true);
        }
      },
    );
    return () => {
      active = false;
    };
  }, []);

  const applyMarkers = (editor: MonacoEditor | null) => {
    const model = editor?.getModel();
    if (!monacoModule || !model) {
      return;
    }
    const lineCount = model.getLineCount();
    monacoModule.monaco.editor.setModelMarkers(
      model,
      markerOwner,
      diagnostics.map((diagnostic) => {
        const line = Math.min(Math.max(diagnostic.line, 1), lineCount);
        return {
          endColumn: model.getLineMaxColumn(line),
          endLineNumber: line,
          message: diagnostic.message,
          severity: monacoModule.monaco.MarkerSeverity.Error,
          startColumn: model.getLineFirstNonWhitespaceColumn(line) || 1,
          startLineNumber: line,
        };
      }),
    );
  };

  useEffect(() => {
    applyMarkers(editorRef.current);
  });

  if (loadFailed) {
    return (
      <TextArea
        aria-describedby={describedBy}
        aria-label={ariaLabel}
        className="pf-v6-u-font-family-monospace"
        id={id}
        onChange={(_event, value) => {
          onChange(value);
        }}
        resizeOrientation="vertical"
        rows={Math.max(code.split("\n").length, 8)}
        validated={diagnostics.length ? "error" : "default"}
        value={code}
      />
    );
  }

  const placeholder = <CedarCodeBlock source={code} />;

  if (!monacoModule) {
    return placeholder;
  }

  return (
    <div
      aria-describedby={describedBy}
      className={diagnostics.length ? styles.invalid : undefined}
      id={id}
    >
      <CodeEditor
        code={code}
        editorProps={{ language: monacoModule.cedarLanguageId }}
        height={`${String(Math.max(editorHeightPixels(code), contentHeight))}px`}
        isLineNumbersVisible
        loading={placeholder}
        onChange={(value) => {
          onChange(value);
        }}
        onEditorDidMount={(editor) => {
          editorRef.current = editor;
          applyMarkers(editor);
          editor.onDidContentSizeChange((event) => {
            setContentHeight(Math.ceil(event.contentHeight) + 16);
          });
        }}
        options={{
          ariaLabel,
          automaticLayout: true,
          fixedOverflowWidgets: true,
          fontSize: editorFontSize,
          lineHeight: editorLineHeight,
          minimap: { enabled: false },
          scrollBeyondLastLine: false,
          wordWrap: "on",
        }}
      />
    </div>
  );
}
