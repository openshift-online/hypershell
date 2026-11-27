import { useRef, type KeyboardEvent, type PointerEvent } from "react";
import { useIntl } from "react-intl";

import {
  terminalResizeStep,
  type TerminalDock,
} from "../../application/terminal/terminal-layout";
import { terminalMessages } from "./terminal-messages";
import styles from "./terminal.module.css";

export interface TerminalResizeHandleProps {
  controlsId: string;
  dock: TerminalDock;
  max: number;
  min: number;
  /** `commit` is true when the size should be saved. */
  onResize: (size: number, commit: boolean) => void;
  size: number;
}

interface DragState {
  pointerId: number;
  startPosition: number;
  startSize: number;
}

/**
 * The drawer edge next to the page (WEB-TERM-03): a WAI-ARIA window splitter
 * whose value is the drawer size in CSS pixels. Pointer drag and the arrow
 * keys resize it in 24 px steps, clamped to the dock's bounds.
 */
export function TerminalResizeHandle({
  controlsId,
  dock,
  max,
  min,
  onResize,
  size,
}: TerminalResizeHandleProps) {
  const intl = useIntl();
  const dragRef = useRef<DragState | null>(null);
  const clamp = (value: number) => Math.min(max, Math.max(min, value));
  const grow = dock === "bottom" ? "ArrowUp" : "ArrowLeft";
  const shrink = dock === "bottom" ? "ArrowDown" : "ArrowRight";

  const onKeyDown = (event: KeyboardEvent<HTMLDivElement>) => {
    let next: number | null = null;
    if (event.key === grow) {
      next = size + terminalResizeStep;
    } else if (event.key === shrink) {
      next = size - terminalResizeStep;
    } else if (event.key === "Home") {
      next = min;
    } else if (event.key === "End") {
      next = max;
    }
    if (next !== null) {
      event.preventDefault();
      onResize(clamp(next), true);
    }
  };

  const positionOf = (event: PointerEvent<HTMLDivElement>) =>
    dock === "bottom" ? event.clientY : event.clientX;

  const sizeFor = (event: PointerEvent<HTMLDivElement>) => {
    const drag = dragRef.current;
    if (drag === null) {
      return size;
    }
    // The drawer grows as the handle moves up (bottom) or left (right).
    return clamp(drag.startSize - (positionOf(event) - drag.startPosition));
  };

  const endDrag = (event: PointerEvent<HTMLDivElement>) => {
    if (dragRef.current?.pointerId !== event.pointerId) {
      return;
    }
    onResize(sizeFor(event), true);
    dragRef.current = null;
    try {
      event.currentTarget.releasePointerCapture(event.pointerId);
    } catch {
      // The capture may already be gone.
    }
  };

  return (
    // A focusable separator is a widget (WAI-ARIA window splitter pattern),
    // which jsx-a11y classifies as non-interactive.
    // eslint-disable-next-line jsx-a11y/no-noninteractive-element-interactions
    <div
      aria-controls={controlsId}
      aria-label={intl.formatMessage(terminalMessages.resizeHandle)}
      aria-orientation={dock === "bottom" ? "horizontal" : "vertical"}
      aria-valuemax={max}
      aria-valuemin={min}
      aria-valuenow={size}
      aria-valuetext={intl.formatMessage(terminalMessages.resizeValue, {
        size,
      })}
      className={[
        styles.handle,
        dock === "bottom" ? styles.handleBottom : styles.handleRight,
      ].join(" ")}
      onKeyDown={onKeyDown}
      onLostPointerCapture={() => {
        dragRef.current = null;
      }}
      onPointerCancel={endDrag}
      onPointerDown={(event) => {
        if (event.button !== 0) {
          return;
        }
        event.preventDefault();
        dragRef.current = {
          pointerId: event.pointerId,
          startPosition: positionOf(event),
          startSize: size,
        };
        try {
          event.currentTarget.setPointerCapture(event.pointerId);
        } catch {
          // Pointer capture is unavailable; the drag still ends on pointerup.
        }
      }}
      onPointerMove={(event) => {
        if (dragRef.current?.pointerId === event.pointerId) {
          onResize(sizeFor(event), false);
        }
      }}
      onPointerUp={endDrag}
      role="separator"
      // A focusable separator is a widget (WAI-ARIA window splitter pattern),
      // which jsx-a11y classifies as non-interactive.
      // eslint-disable-next-line jsx-a11y/no-noninteractive-tabindex
      tabIndex={0}
    />
  );
}
