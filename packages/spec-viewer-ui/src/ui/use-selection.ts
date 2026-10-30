import { useCallback, useEffect, useState } from "react";

export interface Selection {
  nodeId: string;
  /** Changes on every selection, so re-selecting the same node flashes again. */
  revision: number;
}

function readHash(): string | null {
  const hash = window.location.hash.slice(1);
  if (!hash) return null;
  try {
    return decodeURIComponent(hash);
  } catch {
    return null;
  }
}

/** Selected spec node, mirrored into the URL hash so it survives reloads. */
export function useSelection(): [Selection | null, (nodeId: string) => void] {
  const [selection, setSelection] = useState<Selection | null>(() => {
    const nodeId = readHash();
    return nodeId ? { nodeId, revision: 0 } : null;
  });

  useEffect(() => {
    const onHashChange = () => {
      const nodeId = readHash();
      if (nodeId) {
        setSelection((current) =>
          current?.nodeId === nodeId
            ? current
            : { nodeId, revision: (current?.revision ?? 0) + 1 },
        );
      }
    };
    window.addEventListener("hashchange", onHashChange);
    return () => {
      window.removeEventListener("hashchange", onHashChange);
    };
  }, []);

  const select = useCallback((nodeId: string) => {
    window.history.pushState(null, "", `#${encodeURIComponent(nodeId)}`);
    setSelection((current) => ({
      nodeId,
      revision: (current?.revision ?? 0) + 1,
    }));
  }, []);

  return [selection, select];
}
