import {
  ClipboardCopyButton,
  CodeBlock,
  CodeBlockAction,
  CodeBlockCode,
} from "@patternfly/react-core";
import { type SyntheticEvent, useEffect, useId, useRef, useState } from "react";
import { useIntl } from "react-intl";

import { messages } from "../messages";
import { type CommandPart, highlightTemplate } from "./command-highlight";
import styles from "./gateway-connection-steps.module.css";

// Editable field values are shell identifiers (provider, sandbox, model names),
// so restrict typed/pasted input to the same safe argument charset the command
// builder uses -- this keeps the displayed value copy-paste safe and identical
// to what the copy button emits (no quoting ever needed).
const disallowed = /[^A-Za-z0-9_./:@%+=,-]+/g;

export function sanitizeFieldValue(value: string): string {
  return value.replace(disallowed, "");
}

function moveCaretToEnd(element: HTMLElement): void {
  const selection = window.getSelection();
  if (!selection) {
    return;
  }
  try {
    selection.selectAllChildren(element);
    selection.collapseToEnd();
  } catch {
    // Some environments (e.g. jsdom) do not implement Selection ranges; the
    // caret position is a best-effort nicety, so ignore failures here.
  }
}

/** An inline value slot in the command that the operator can edit in place. */
function EditableField({
  colorClassName,
  label,
  onChange,
  value,
}: {
  colorClassName: string;
  label: string;
  onChange: (value: string) => void;
  value: string;
}) {
  const ref = useRef<HTMLSpanElement>(null);
  // Render the text exactly once as the element's initial content; thereafter
  // React must not reconcile the children (that would fight the user's edits),
  // so the DOM is the source of truth and external updates are applied by hand.
  // Captured in state (not a ref) so it can be read during render.
  const [initial] = useState(value);

  // When the value changes elsewhere -- e.g. the mirrored provider slot in the
  // other half of the command -- push it into this field's DOM, but never while
  // it is the one being edited (that would move the caret out from under typing).
  useEffect(() => {
    const element = ref.current;
    if (
      element &&
      document.activeElement !== element &&
      element.textContent !== value
    ) {
      element.textContent = value;
    }
  }, [value]);

  const handleInput = (event: SyntheticEvent<HTMLSpanElement>) => {
    const element = event.currentTarget;
    const raw = element.textContent;
    const clean = sanitizeFieldValue(raw);
    if (clean !== raw) {
      // Rewrite the DOM to drop the rejected character and drop the caret at the
      // end, so what is shown stays exactly what gets copied.
      element.textContent = clean;
      moveCaretToEnd(element);
    }
    onChange(clean);
  };

  return (
    <span
      aria-label={label}
      className={[styles.field, colorClassName].filter(Boolean).join(" ")}
      contentEditable="plaintext-only"
      onInput={handleInput}
      onKeyDown={(event) => {
        if (event.key === "Enter") {
          event.preventDefault();
        }
      }}
      ref={ref}
      role="textbox"
      spellCheck={false}
      suppressContentEditableWarning
      tabIndex={0}
    >
      {initial}
    </span>
  );
}

/**
 * A copyable command block whose marked value slots are editable in place.
 *
 * `templateCommand` carries the edit markers and is highlighted once; `copyText`
 * is the same command with the operator's current values resolved and drives
 * both the copy button and (identically) a whole-block text selection. Editing a
 * field calls `onFieldChange(marker, value)`; a marker used twice in the command
 * (the mirrored provider name) is kept in lockstep because both slots read the
 * same entry in `values`.
 */
export function EditableCommand({
  copyAriaLabel,
  copyText,
  labels,
  markers,
  onFieldChange,
  templateCommand,
  values,
}: {
  copyAriaLabel: string;
  copyText: string;
  labels: Record<string, string>;
  markers: readonly string[];
  onFieldChange: (marker: string, value: string) => void;
  templateCommand: string;
  values: Record<string, string>;
}) {
  const intl = useIntl();
  const id = useId();
  const [copied, setCopied] = useState(false);
  const [parts, setParts] = useState<CommandPart[]>();

  // Highlight asynchronously so the plain command renders immediately and the
  // themed, editable markup swaps in once Shiki resolves. The template (marker
  // form) is stable, so this runs once per command shape.
  useEffect(() => {
    let active = true;
    void highlightTemplate(templateCommand, markers).then((next) => {
      if (active) {
        setParts(next);
      }
    });
    return () => {
      active = false;
    };
  }, [templateCommand, markers]);

  const handleCopy = () => {
    void navigator.clipboard.writeText(copyText);
    setCopied(true);
  };

  return (
    <CodeBlock
      actions={
        <CodeBlockAction>
          <ClipboardCopyButton
            aria-label={copyAriaLabel}
            exitDelay={copied ? 1500 : 600}
            id={`${id}-copy-button`}
            maxWidth="110px"
            onClick={handleCopy}
            onTooltipHidden={() => {
              setCopied(false);
            }}
            variant="plain"
          >
            {copied
              ? intl.formatMessage(messages.copied)
              : intl.formatMessage(messages.copy)}
          </ClipboardCopyButton>
        </CodeBlockAction>
      }
    >
      {parts ? (
        <div className={styles.highlighted}>
          <pre className="shiki">
            <code>
              {parts.map((part, index) =>
                part.kind === "text" ? (
                  <span className={part.className} key={index}>
                    {part.value}
                  </span>
                ) : (
                  <EditableField
                    colorClassName={part.className}
                    key={index}
                    label={labels[part.marker] ?? ""}
                    onChange={(value) => {
                      onFieldChange(part.marker, value);
                    }}
                    value={values[part.marker] ?? ""}
                  />
                ),
              )}
            </code>
          </pre>
        </div>
      ) : (
        <CodeBlockCode id={id}>{copyText}</CodeBlockCode>
      )}
    </CodeBlock>
  );
}
