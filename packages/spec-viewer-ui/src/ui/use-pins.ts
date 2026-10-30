import { useCallback, useState } from "react";

import { usePorts } from "./ports-context";

export interface Pins {
  ids: string[];
  isPinned: (nodeId: string) => boolean;
  pin: (nodeIds: string[]) => void;
  unpin: (nodeIds: string[]) => void;
  clear: () => void;
}

export function usePins(): Pins {
  const { pins: store } = usePorts();
  const [ids, setIds] = useState<string[]>(() => store.load());

  const update = useCallback(
    (next: (current: string[]) => string[]) => {
      setIds((current) => {
        const value = next(current);
        store.save(value);
        return value;
      });
    },
    [store],
  );

  return {
    ids,
    isPinned: (nodeId) => ids.includes(nodeId),
    pin: (nodeIds) => {
      update((current) => [
        ...current,
        ...nodeIds.filter((id, index) => {
          return !current.includes(id) && nodeIds.indexOf(id) === index;
        }),
      ]);
    },
    unpin: (nodeIds) => {
      update((current) => current.filter((id) => !nodeIds.includes(id)));
    },
    clear: () => {
      update(() => []);
    },
  };
}
