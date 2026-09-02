import {
  Dropdown,
  DropdownItem,
  DropdownList,
  MenuToggle,
} from "@patternfly/react-core";
import { UserIcon } from "@patternfly/react-icons";
import { useState } from "react";
import { FormattedMessage, useIntl } from "react-intl";

import { readBrowserRuntimeConfig } from "../../composition/browser-runtime-config";
import { messages } from "../../i18n/messages";
import { useSession } from "./use-session";

/**
 * Masthead identity menu. Shows the authenticated user's display name and a
 * single sign-out action that performs full RP-initiated logout by navigating
 * to the BFF `/auth/logout` endpoint (a real navigation, not a client route).
 * Renders nothing when unauthenticated or in no-auth mode.
 */
export function UserMenu() {
  const intl = useIntl();
  const [isOpen, setIsOpen] = useState(false);
  const { data: session } = useSession();
  const buildVersion =
    readBrowserRuntimeConfig().build.version ??
    intl.formatMessage(messages.unknownVersion);

  if (!session?.authenticated) {
    return null;
  }

  const displayName =
    session.user?.name ??
    session.user?.preferredUsername ??
    session.user?.email ??
    intl.formatMessage(messages.account);

  return (
    <Dropdown
      isOpen={isOpen}
      onOpenChange={setIsOpen}
      onSelect={() => {
        setIsOpen(false);
      }}
      popperProps={{ position: "right" }}
      shouldFocusToggleOnSelect
      toggle={(toggleRef) => (
        <MenuToggle
          icon={<UserIcon aria-hidden="true" />}
          isExpanded={isOpen}
          onClick={() => {
            setIsOpen((open) => !open);
          }}
          ref={toggleRef}
        >
          {displayName}
        </MenuToggle>
      )}
    >
      <DropdownList>
        <DropdownItem isDisabled>
          <FormattedMessage
            {...messages.consoleVersion}
            values={{ version: buildVersion }}
          />
        </DropdownItem>
        <DropdownItem to="/auth/logout">
          <FormattedMessage {...messages.logout} />
        </DropdownItem>
      </DropdownList>
    </Dropdown>
  );
}
