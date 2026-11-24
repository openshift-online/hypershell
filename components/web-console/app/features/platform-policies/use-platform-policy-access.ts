import { useQuery } from "@tanstack/react-query";

import { platformPolicyOperations } from "../../composition/platform-policy-composition";
import { useSession } from "../shell/use-session";
import { platformPolicyQueryKeys } from "./platform-policy-query-keys";

/**
 * Whether the caller may administer platform policies (WEB-PADM-01).
 *
 * `platform-policy:admin` is a database RoleBinding granted through the
 * RoleBinding API, not a JWT realm role (SEC-RBAC-30), so the session's roles
 * cannot answer this. The console asks the enforcement point instead: `GET
 * /api/hypershell/ext/platform_policy_settings` is authorized for exactly
 * `platform-policy:admin` and `platform:admin`, so `200` means authorized and
 * `403` means hide. Hiding is presentation only; every write is still
 * authorized by the API server.
 */
export function usePlatformPolicyAccess() {
  const { data: session, isPending: isSessionPending } = useSession();
  const signedIn = session
    ? !session.authEnabled || session.authenticated
    : false;
  const access = useQuery({
    enabled: signedIn,
    queryFn: ({ signal }) => platformPolicyOperations.checkAccess(signal),
    queryKey: platformPolicyQueryKeys.access(),
    retry: false,
    staleTime: 300_000,
  });
  return {
    isAuthorized: access.data === "authorized",
    isError: access.isError,
    isPending: isSessionPending || (signedIn && access.isPending),
    refetch: access.refetch,
  };
}
