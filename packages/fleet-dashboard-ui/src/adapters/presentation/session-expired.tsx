// Full-page takeover shown when the user's session has gone stale (the BFF returned
// 401). Instead of every data panel showing an identical generic error, we replace the
// whole view with a single clear message and a "Sign in again" action. The button reloads
// the top-level document so the oauth-proxy re-runs its identity-provider sign-in (the
// HTTP-only session cookie it owns can't be cleared from JS - see adapters/auth/session-expiry).

import {
  Bullseye,
  Button,
  Content,
  Stack,
  StackItem,
} from "@patternfly/react-core";
import ExclamationCircleIcon from "@patternfly/react-icons/dist/esm/icons/exclamation-circle-icon";
import { FormattedMessage } from "react-intl";

import { signInAgain } from "../auth/session-expiry";
import { messages } from "../../messages";

export function SessionExpired(): React.ReactElement {
  return (
    <Bullseye style={{ minHeight: "100vh" }}>
      <Stack hasGutter style={{ maxWidth: 440, textAlign: "center" }}>
        <StackItem>
          <ExclamationCircleIcon
            style={{
              fontSize: 40,
              color:
                "var(--pf-t--global--icon--color--status--danger--default, #c9461e)",
            }}
          />
        </StackItem>
        <StackItem>
          <Content component="h1">
            <FormattedMessage {...messages.sessionExpiredTitle} />
          </Content>
        </StackItem>
        <StackItem>
          <Content component="p">
            <FormattedMessage {...messages.sessionExpiredBody} />
          </Content>
        </StackItem>
        <StackItem>
          <Button variant="primary" onClick={signInAgain}>
            <FormattedMessage {...messages.sessionSignIn} />
          </Button>
        </StackItem>
      </Stack>
    </Bullseye>
  );
}
