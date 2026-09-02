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
import { useApiVersion } from "./use-api-version";
import { useSession } from "./use-session";

/**
 * Masthead identity menu. Shows the user's display name, the console and API
 * image versions, and a sign-out action. Sign-out uses the BFF `/auth/logout`
 * endpoint so that the BFF can complete RP-initiated logout. The menu renders
 * nothing when the user is not authenticated or when no-auth mode is active.
 */
export function UserMenu() {
  const intl = useIntl();
  const [isOpen, setIsOpen] = useState(false);
  const [configuredBuildVersion] = useState(
    () => readBrowserRuntimeConfig().build.version,
  );
  const { data: session } = useSession();
  const { data: apiBuildVersion } = useApiVersion(
    session?.authenticated === true,
  );
  const unknownVersion = intl.formatMessage(messages.unknownVersion);
  const consoleBuildVersion = configuredBuildVersion ?? unknownVersion;
  const displayedApiBuildVersion = apiBuildVersion ?? unknownVersion;

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
            values={{ version: consoleBuildVersion }}
          />
        </DropdownItem>
        <DropdownItem isDisabled>
          <FormattedMessage
            {...messages.apiVersion}
            values={{ version: displayedApiBuildVersion }}
          />
        </DropdownItem>
        <DropdownItem to="/auth/logout">
          <FormattedMessage {...messages.logout} />
        </DropdownItem>
      </DropdownList>
    </Dropdown>
  );
}
