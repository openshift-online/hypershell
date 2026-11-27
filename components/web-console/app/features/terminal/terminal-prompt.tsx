import {
  InputGroup,
  InputGroupItem,
  InputGroupText,
  TextInput,
} from "@patternfly/react-core";
import {
  useId,
  useLayoutEffect,
  useRef,
  useState,
  type KeyboardEvent,
  type RefObject,
} from "react";
import { useIntl } from "react-intl";

import {
  deleteToLineEnd,
  deleteToLineStart,
  deleteWordBefore,
  initialCommandRecall,
  recallCommand,
  type LineEditState,
} from "../../application/terminal/line-editing";
import { terminalMessages } from "./terminal-messages";
import styles from "./terminal.module.css";

export interface TerminalPromptProps {
  /** False while the runtime loads: Enter does not run a command. */
  acceptsCommands: boolean;
  commandHistory: readonly string[];
  inputRef: RefObject<HTMLInputElement | null>;
  isRunning: boolean;
  onCancel: () => void;
  onChange: (value: string) => void;
  onClear: () => void;
  onFocusChange: (focused: boolean) => void;
  onInterrupt: (line: string) => void;
  /** Returns false when the line was not accepted (a command is running). */
  onSubmit: (line: string) => boolean;
  value: string;
}

/** Shell prompt symbol; a symbol, not translatable text. */
const promptSymbol = "$";

function hasOnlyCtrl(event: KeyboardEvent): boolean {
  return event.ctrlKey && !event.altKey && !event.metaKey;
}

function hasNoModifier(event: KeyboardEvent): boolean {
  return !event.ctrlKey && !event.altKey && !event.metaKey && !event.shiftKey;
}

/**
 * The terminal's single text input with line-editing keys (WEB-TERM-05).
 * Keys are handled only while this input has focus, and each handled key
 * prevents the browser default when the browser delivers it to the page.
 */
export function TerminalPrompt({
  acceptsCommands,
  commandHistory,
  inputRef,
  isRunning,
  onCancel,
  onChange,
  onClear,
  onFocusChange,
  onInterrupt,
  onSubmit,
  value,
}: TerminalPromptProps) {
  const intl = useIntl();
  const helpId = useId();
  const [recall, setRecall] = useState(initialCommandRecall);
  const pendingCursorRef = useRef<number | null>(null);

  useLayoutEffect(() => {
    const cursor = pendingCursorRef.current;
    const input = inputRef.current;
    if (cursor !== null && input !== null) {
      pendingCursorRef.current = null;
      input.setSelectionRange(cursor, cursor);
    }
  }, [inputRef, value]);

  const edit = (state: LineEditState) => {
    pendingCursorRef.current = state.cursor;
    setRecall(initialCommandRecall);
    onChange(state.value);
  };

  const onKeyDown = (event: KeyboardEvent<HTMLInputElement>) => {
    const input = event.currentTarget;
    const cursor = input.selectionStart ?? value.length;
    const state = { cursor, value };
    const key = event.key.length === 1 ? event.key.toLowerCase() : event.key;

    if (event.key === "Enter" && hasNoModifier(event)) {
      event.preventDefault();
      if (acceptsCommands && onSubmit(value)) {
        setRecall(initialCommandRecall);
        onChange("");
      }
      return;
    }
    if (hasOnlyCtrl(event)) {
      switch (key) {
        case "l":
          event.preventDefault();
          onClear();
          return;
        case "u":
          event.preventDefault();
          edit(deleteToLineStart(state));
          return;
        case "k":
          event.preventDefault();
          edit(deleteToLineEnd(state));
          return;
        case "a":
          event.preventDefault();
          input.setSelectionRange(0, 0);
          return;
        case "e":
          event.preventDefault();
          input.setSelectionRange(value.length, value.length);
          return;
        case "c":
          event.preventDefault();
          if (isRunning) {
            onCancel();
          } else {
            onInterrupt(value);
            setRecall(initialCommandRecall);
            onChange("");
          }
          return;
        case "w":
        case "Backspace":
          event.preventDefault();
          edit(deleteWordBefore(state));
          return;
        default:
          return;
      }
    }
    if (
      event.altKey &&
      !event.ctrlKey &&
      !event.metaKey &&
      event.key === "Backspace"
    ) {
      event.preventDefault();
      edit(deleteWordBefore(state));
      return;
    }
    if (
      (event.key === "ArrowUp" || event.key === "ArrowDown") &&
      hasNoModifier(event)
    ) {
      const recalled = recallCommand(
        commandHistory,
        recall,
        value,
        event.key === "ArrowUp" ? "older" : "newer",
      );
      if (recalled !== null) {
        event.preventDefault();
        setRecall(recalled.state);
        pendingCursorRef.current = recalled.value.length;
        onChange(recalled.value);
      }
    }
  };

  return (
    <div className={styles.promptRow}>
      <InputGroup>
        <InputGroupText aria-hidden="true" className={styles.input}>
          {promptSymbol}
        </InputGroupText>
        <InputGroupItem isFill>
          <TextInput
            aria-describedby={helpId}
            aria-label={intl.formatMessage(terminalMessages.inputLabel)}
            autoCapitalize="off"
            autoComplete="off"
            className={styles.input}
            onBlur={() => {
              onFocusChange(false);
            }}
            onChange={(_event, next) => {
              setRecall(initialCommandRecall);
              onChange(next);
            }}
            onFocus={() => {
              onFocusChange(true);
            }}
            onKeyDown={onKeyDown}
            readOnly={!acceptsCommands}
            ref={inputRef}
            spellCheck={false}
            type="text"
            value={value}
          />
        </InputGroupItem>
      </InputGroup>
      <p className={styles.hint} id={helpId}>
        {intl.formatMessage(
          isRunning ? terminalMessages.running : terminalMessages.inputHelp,
        )}
      </p>
    </div>
  );
}
