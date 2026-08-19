import { useQuery } from "@tanstack/react-query";

import { sessionGateway } from "../../composition/session-composition";
import type { BrowserSession } from "../../adapters/session/session-adapter";

export const sessionQueryKey = ["session"] as const;

/**
 * Reads the BFF browser session resource. Identity is stable for a session, so
 * it is cached generously; re-authentication on expiry is handled by the API
 * transport, not by polling this resource.
 */
export function useSession() {
  return useQuery<BrowserSession>({
    queryFn: ({ signal }) => sessionGateway.getSession(signal),
    queryKey: sessionQueryKey,
    staleTime: 300_000,
  });
}
