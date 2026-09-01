import {
  Dropdown,
  DropdownItem,
  DropdownList,
  MenuToggle,
} from "@patternfly/react-core";
import { EllipsisVIcon } from "@patternfly/react-icons";
import { useState } from "react";
import { FormattedMessage, useIntl } from "react-intl";

import type { GatewayProfileRecord } from "../application/gateway-profile-types";
import { gatewayProfileMessages } from "../gateway-profile-messages";
import { GatewayProfileDeleteDialog } from "./gateway-profile-delete-dialog";

export function GatewayProfileRowActions({
  gatewayProfile,
  onDeleted,
}: {
  gatewayProfile: GatewayProfileRecord;
  onDeleted: () => void;
}) {
  const intl = useIntl();
  const [isDeleteOpen, setIsDeleteOpen] = useState(false);
  const [isOpen, setIsOpen] = useState(false);

  return (
    <>
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
            aria-label={intl.formatMessage(
              gatewayProfileMessages.gatewayProfileRowActions,
              { gatewayProfileName: gatewayProfile.name },
            )}
            isExpanded={isOpen}
            onClick={() => {
              setIsOpen((open) => !open);
            }}
            ref={toggleRef}
            variant="plain"
          >
            <EllipsisVIcon aria-hidden="true" />
          </MenuToggle>
        )}
      >
        <DropdownList>
          <DropdownItem
            isDanger
            onClick={() => {
              setIsDeleteOpen(true);
            }}
          >
            <FormattedMessage
              {...gatewayProfileMessages.deleteGatewayProfile}
            />
          </DropdownItem>
        </DropdownList>
      </Dropdown>
      <GatewayProfileDeleteDialog
        gatewayProfileId={gatewayProfile.id}
        gatewayProfileName={gatewayProfile.name}
        isOpen={isDeleteOpen}
        onClose={() => {
          setIsDeleteOpen(false);
        }}
        onDeleted={() => {
          setIsDeleteOpen(false);
          onDeleted();
        }}
      />
    </>
  );
}
