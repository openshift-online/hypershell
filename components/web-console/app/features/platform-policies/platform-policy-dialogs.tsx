import {
  Alert,
  AlertActionCloseButton,
  Button,
  Content,
  Modal,
  ModalBody,
  ModalFooter,
  ModalHeader,
  ModalVariant,
  Stack,
  StackItem,
} from "@patternfly/react-core";
import { Table, Tbody, Td, Th, Thead, Tr } from "@patternfly/react-table";
import type { MouseEvent } from "react";
import { useIntl } from "react-intl";

import type { PlatformPolicy } from "../../application/platform-policies/platform-policy-types";
import { platformPoliciesMessages as messages } from "./platform-policies-messages";
import {
  enforcementImpact,
  type GatewayImpact,
  type PlatformPolicyDialog,
  type PlatformPolicyWriteFailure,
} from "./platform-policy-summary";

/** Link props that route in-app on a plain primary click. */
export function navigationLinkProps(
  href: string,
  onNavigate: (href: string) => void,
) {
  return {
    href,
    onClick: (event: MouseEvent<HTMLElement>) => {
      if (
        event.defaultPrevented ||
        event.button !== 0 ||
        event.metaKey ||
        event.ctrlKey ||
        event.shiftKey ||
        event.altKey
      ) {
        return;
      }
      event.preventDefault();
      onNavigate(href);
    },
  };
}

function ImpactTable({ rows }: { rows: readonly GatewayImpact[] }) {
  const intl = useIntl();
  if (rows.length === 0) {
    return (
      <Content component="p">
        {intl.formatMessage(messages.dialogImpactNone)}
      </Content>
    );
  }
  const gatewayHeading = intl.formatMessage(messages.columnGateway);
  const rejectHeading = intl.formatMessage(messages.columnWouldRejectShort);
  const noteHeading = intl.formatMessage(messages.columnNote);
  return (
    <Table
      aria-label={intl.formatMessage(messages.dialogImpactTable)}
      variant="compact"
    >
      <Thead>
        <Tr>
          <Th>{gatewayHeading}</Th>
          <Th>{rejectHeading}</Th>
          <Th>{noteHeading}</Th>
        </Tr>
      </Thead>
      <Tbody>
        {rows.map((row) => {
          let note = intl.formatMessage(messages.notApplicable);
          if (row.note === "pending") {
            note = intl.formatMessage(messages.noteRevisionPending);
          } else if (row.note === "unsupported") {
            note = row.message
              ? intl.formatMessage(messages.noteCannotEnforce, {
                  message: row.message,
                })
              : intl.formatMessage(messages.noteCannotEnforceNoReason);
          }
          return (
            <Tr key={row.gatewayId}>
              <Td dataLabel={gatewayHeading}>{row.label}</Td>
              <Td dataLabel={rejectHeading}>{row.wouldReject}</Td>
              <Td dataLabel={noteHeading}>{note}</Td>
            </Tr>
          );
        })}
      </Tbody>
    </Table>
  );
}

/**
 * Confirmation dialogs for the global toggle, a policy's own mode, and
 * deletion. Both mode directions require confirmation; raising to Enforced is
 * the higher-consequence action and shows the 24-hour impact by gateway and an
 * explicit gateway count on the confirm button (WEB-PADM-03, UIS-CONT-03).
 */
export function PlatformPolicyDialogs({
  dialog,
  isWriting,
  onClose,
  onConfirm,
  policies,
}: {
  dialog?: PlatformPolicyDialog;
  isWriting: boolean;
  onClose: () => void;
  onConfirm: (dialog: PlatformPolicyDialog) => void;
  policies: readonly PlatformPolicy[];
}) {
  const intl = useIntl();
  if (!dialog) {
    return null;
  }
  const policy =
    dialog.kind === "global-mode"
      ? undefined
      : policies.find(({ id }) => id === dialog.policyId);
  if (dialog.kind !== "global-mode" && !policy) {
    return null;
  }
  const scope = policy ? [policy] : policies;
  const impact = enforcementImpact(scope);
  const cancel = (
    <Button isDisabled={isWriting} onClick={onClose} variant="link">
      {intl.formatMessage(messages.cancel)}
    </Button>
  );
  const confirm = (label: string, variant: "danger" | "primary") => (
    <Button
      isDisabled={isWriting}
      isLoading={isWriting}
      onClick={() => {
        onConfirm(dialog);
      }}
      variant={variant}
    >
      {label}
    </Button>
  );

  if (dialog.kind === "delete") {
    return (
      <Modal
        aria-labelledby="platform-policy-dialog-title"
        isOpen
        onClose={onClose}
        variant={ModalVariant.small}
      >
        <ModalHeader
          labelId="platform-policy-dialog-title"
          title={intl.formatMessage(messages.dialogDeleteTitle, {
            name: policy?.name ?? "",
          })}
          titleIconVariant="warning"
        />
        <ModalBody>
          {intl.formatMessage(messages.dialogDeleteBody, {
            count: impact.gatewayCount,
          })}
        </ModalBody>
        <ModalFooter>
          {confirm(intl.formatMessage(messages.deletePolicy), "danger")}
          {cancel}
        </ModalFooter>
      </Modal>
    );
  }

  if (dialog.to === "audit") {
    return (
      <Modal
        aria-labelledby="platform-policy-dialog-title"
        isOpen
        onClose={onClose}
        variant={ModalVariant.small}
      >
        <ModalHeader
          labelId="platform-policy-dialog-title"
          title={
            policy
              ? intl.formatMessage(messages.dialogAuditPolicyTitle, {
                  name: policy.name,
                })
              : intl.formatMessage(messages.dialogAuditAllTitle)
          }
          titleIconVariant="warning"
        />
        <ModalBody>
          {intl.formatMessage(
            policy
              ? messages.dialogAuditPolicyBody
              : messages.dialogAuditAllBody,
            { count: impact.gatewayCount },
          )}
        </ModalBody>
        <ModalFooter>
          {confirm(
            intl.formatMessage(
              policy
                ? messages.dialogAuditPolicyConfirm
                : messages.dialogAuditAllConfirm,
            ),
            "primary",
          )}
          {cancel}
        </ModalFooter>
      </Modal>
    );
  }

  return (
    <Modal
      aria-labelledby="platform-policy-dialog-title"
      isOpen
      onClose={onClose}
      variant={ModalVariant.medium}
    >
      <ModalHeader
        labelId="platform-policy-dialog-title"
        title={
          policy
            ? intl.formatMessage(messages.dialogStartEnforcingTitle, {
                name: policy.name,
              })
            : intl.formatMessage(messages.dialogEnforceAllTitle, {
                count: impact.gatewayCount,
              })
        }
        titleIconVariant="warning"
      />
      <ModalBody>
        <Stack hasGutter>
          <StackItem>
            {intl.formatMessage(
              policy
                ? messages.dialogStartEnforcingBody
                : messages.dialogEnforceAllBody,
            )}
          </StackItem>
          <StackItem>
            <Alert
              isInline
              title={intl.formatMessage(messages.dialogImpactTitle, {
                count: impact.wouldReject,
              })}
              variant="warning"
            >
              {intl.formatMessage(messages.dialogImpactBody, {
                count: impact.wouldAddDefaults,
              })}
            </Alert>
          </StackItem>
          <StackItem>
            <ImpactTable rows={impact.rows} />
          </StackItem>
        </Stack>
      </ModalBody>
      <ModalFooter>
        {confirm(
          policy
            ? intl.formatMessage(messages.dialogStartEnforcingConfirm)
            : intl.formatMessage(messages.dialogEnforceAllConfirm, {
                count: impact.gatewayCount,
              }),
          "primary",
        )}
        {cancel}
      </ModalFooter>
    </Modal>
  );
}

/**
 * A refused or failed write. A `403` is a denied operation, not an error to
 * retry, so no retry action is offered (WEB-PADM-01.02).
 */
export function PlatformPolicyWriteAlert({
  failure,
  onDismiss,
}: {
  failure?: PlatformPolicyWriteFailure;
  onDismiss: () => void;
}) {
  const intl = useIntl();
  if (!failure) {
    return null;
  }
  const copy = {
    conflicted: [messages.writeConflictTitle, messages.writeConflictBody],
    denied: [messages.writeDeniedTitle, messages.writeDeniedBody],
    failed: [messages.writeFailedTitle, messages.writeFailedBody],
  } as const;
  const [title, body] = copy[failure];
  return (
    <Alert
      actionClose={
        <AlertActionCloseButton
          aria-label={intl.formatMessage(messages.dismiss)}
          onClose={onDismiss}
        />
      }
      isInline
      title={intl.formatMessage(title)}
      variant={failure === "failed" ? "danger" : "warning"}
    >
      {intl.formatMessage(body)}
    </Alert>
  );
}
