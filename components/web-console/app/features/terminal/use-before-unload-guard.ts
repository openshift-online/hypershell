import { useEffect } from "react";

/**
 * Registers a `beforeunload` guard only while `active` is true (WEB-TERM-06),
 * so a browser that reserves Ctrl+W asks before closing the tab while the
 * terminal holds unsent input, has focus, or runs a command.
 */
export function useBeforeUnloadGuard(active: boolean): void {
  useEffect(() => {
    if (!active) {
      return;
    }
    const onBeforeUnload = (event: BeforeUnloadEvent) => {
      event.preventDefault();
    };
    window.addEventListener("beforeunload", onBeforeUnload);
    return () => {
      window.removeEventListener("beforeunload", onBeforeUnload);
    };
  }, [active]);
}
