import { useCallback, useState } from "react";

const PREFIX = "hypershell-spec-viewer.width.";

function read(key: string): number | null {
  try {
    const stored = Number(window.localStorage.getItem(PREFIX + key));
    return Number.isFinite(stored) && stored > 0 ? stored : null;
  } catch {
    return null;
  }
}

/**
 * A panel width the user chose by dragging, remembered per browser; null
 * means the default layout.
 */
export function usePanelWidth(
  key: string,
): [number | null, (width: number | null) => void] {
  const [width, setWidthState] = useState<number | null>(() => read(key));
  const setWidth = useCallback(
    (next: number | null) => {
      setWidthState(next);
      try {
        if (next === null) window.localStorage.removeItem(PREFIX + key);
        else window.localStorage.setItem(PREFIX + key, String(next));
      } catch {
        // Widths are a convenience; blocked storage keeps them for the session.
      }
    },
    [key],
  );
  return [width, setWidth];
}
