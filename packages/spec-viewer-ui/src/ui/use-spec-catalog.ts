import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect } from "react";

import { loadSpecCatalog } from "../application/load-catalog";
import { usePorts } from "./ports-context";
import { useSpecRef } from "./ref-context";

const CATALOG_KEY = ["spec-catalog"] as const;
export const TRACEABILITY_KEY = ["traceability"] as const;

/**
 * Loads and parses the specs of the current branch; reloads when a spec file
 * changes on disk.
 */
export function useSpecCatalog() {
  const { specs, markdown } = usePorts();
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

  return useQuery({
    queryKey: [...CATALOG_KEY, ref],
    queryFn: () => loadSpecCatalog(specs, markdown, ref),
    staleTime: Infinity,
  });
}
