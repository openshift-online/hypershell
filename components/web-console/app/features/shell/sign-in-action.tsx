import { Button } from "@patternfly/react-core";
import { FormattedMessage } from "react-intl";
import { useLocation } from "react-router";

import { messages } from "../../i18n/messages";
import { currentPathAndQuery, signInHref } from "../../lib/sign-in";
import { useSession } from "./use-session";

/**
 * Masthead sign-in action shown in place of the user identity when OIDC is
 * configured and the reader has no session, as on a public docs page
 * (WEB-UDOC-03). It is a real navigation to the BFF and returns the reader to
 * the current path and query after sign-in.
 */
export function SignInAction() {
  const location = useLocation();
  const { data: session, isError } = useSession();
  // An unreadable session renders as signed out (WEB-UDOC-03); a cached
  // session keeps deciding after a failed refetch.
  const signedOut =
    session === undefined
      ? isError
      : session.authEnabled && !session.authenticated;
  if (!signedOut) {
    return null;
  }
  return (
    <Button
      component="a"
      href={signInHref(currentPathAndQuery(location))}
      variant="secondary"
    >
      <FormattedMessage {...messages.signIn} />
    </Button>
  );
}
