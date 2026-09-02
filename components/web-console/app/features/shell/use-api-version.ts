import { useQuery } from "@tanstack/react-query";

import { apiVersionReader } from "../../composition/api-version-composition";

export const apiVersionQueryKey = ["api-version"] as const;

/** Reads the API image version after the browser session is authenticated. */
export function useApiVersion(enabled: boolean) {
  return useQuery<string>({
    enabled,
    queryFn: ({ signal }) => apiVersionReader.readVersion(signal),
    queryKey: apiVersionQueryKey,
    staleTime: 300_000,
  });
}
