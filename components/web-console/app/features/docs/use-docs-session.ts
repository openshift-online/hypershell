import { useSession } from "../shell/use-session";

export type DocsSessionState = "loading" | "signed-in" | "signed-out";

/**
 * Session state of a docs reader from the browser session resource
 * (WEB-AUTH-03, WEB-UDOC-03). A resource that cannot be read renders as signed
 * out. In no-auth development mode the console needs no sign-in, so the reader
 * counts as signed in.
 */
export function useDocsSession(): DocsSessionState {
  const { data, isPending } = useSession();
  if (isPending) {
    return "loading";
  }
  if (!data) {
    return "signed-out";
  }
  if (!data.authEnabled) {
    return "signed-in";
  }
  return data.authenticated ? "signed-in" : "signed-out";
}
