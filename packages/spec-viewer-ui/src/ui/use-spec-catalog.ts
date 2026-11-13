import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect } from "react";

import { loadSpecCatalog } from "../application/load-catalog";
import type { RefName } from "../application/ports";
import { usePorts } from "./ports-context";
import { useSpecRef } from "./ref-context";

const CATALOG_KEY = ["spec-catalog"] as const;
export const TRACEABILITY_KEY = ["traceability"] as const;

/**
 * Loads and parses the specs of the current branch; reloads when a spec file
 * changes on disk.
 */
export function useSpecCatalog() {
  const { specs } = usePorts();
  const ref = useSpecRef();
  const queryClient = useQueryClient();

  useEffect(
    () =>
      specs.subscribe(() => {
        void queryClient.invalidateQueries({ queryKey: CATALOG_KEY });
        void queryClient.invalidateQueries({ queryKey: TRACEABILITY_KEY });
      }),
    [specs, queryClient],
  );

  return useCatalogOf(ref);
}

/**
 * Loads and parses the specs of one branch. With `enabled` false nothing is
 * loaded. Reloads with the current branch when a spec file changes.
 */
export function useCatalogOf(ref: RefName, enabled = true) {
  const { specs, markdown } = usePorts();
  return useQuery({
    queryKey: [...CATALOG_KEY, ref],
    queryFn: () => loadSpecCatalog(specs, markdown, ref),
    staleTime: Infinity,
    enabled,
  });
}
