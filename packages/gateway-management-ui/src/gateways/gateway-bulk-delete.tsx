import {
  Alert,
  AlertActionCloseButton,
  AlertActionLink,
  Button,
  Checkbox,
  Content,
  Dropdown,
  DropdownItem,
  DropdownList,
  List,
  ListItem,
  MenuToggle,
  MenuToggleCheckbox,
  Modal,
  ModalBody,
  ModalFooter,
  ModalHeader,
  ModalVariant,
  Tooltip,
} from "@patternfly/react-core";
import { useQueryClient } from "@tanstack/react-query";
import { useCallback, useEffect, useId, useRef, useState } from "react";
import { FormattedMessage, useIntl } from "react-intl";

import { useGatewayUi } from "../gateway-ui-provider";
import { messages } from "../messages";
import { type GatewayConnection } from "./gateway-connections";
import { gatewayListQueryRoot, gatewayQueryKey } from "./gateway-data";

/**
 * Maximum number of concurrent per-gateway delete requests a bulk deletion may
 * have in flight. Bulk deletion issues one request per gateway (WEB-UIEX-08);
 * bounding concurrency avoids an unbounded request burst against the API
 * (`standards/ui/trust-performance.spec.md`).
 */
export const bulkDeleteConcurrencyLimit = 4;

/**
 * A gateway targeted by a bulk deletion. Only the authoritative identity and the
 * display name are needed; eligibility is decided before a target is built.
 */
export interface GatewayBulkDeleteTarget {
  activeSandboxCount?: number;
  id: string;
  name: string;
}

/**
 * The per-gateway result of a bulk deletion. `accepted` means the API accepted
 * the delete request and asynchronous teardown proceeds (it is NOT completed
 * resource cleanup, per `platform/gateway-deletion-finalization.spec.md`);
 * `failed` means the request was rejected and the gateway was not deleted.
 */
export type GatewayBulkDeleteOutcome =
  | { id: string; name: string; status: "accepted" }
  | { error: unknown; id: string; name: string; status: "failed" };

/**
 * Whether a gateway is eligible for deletion. Derived from the authoritative
 * `can_delete` capability (WEB-UIEX-03/06); an absent capability (older server)
 * stays eligible because the API `403` remains the authorization boundary
 * (SEC-RBAC-10). Never inferred from ownership or the ownership filter.
 */
export function isGatewayDeletable(gateway: { canDelete?: boolean }): boolean {
  return gateway.canDelete !== false;
}

/**
 * Deletes each target with at most `limit` requests in flight at once, returning
 * one settled outcome per target in input order. A rejected request is recorded
 * as a `failed` outcome rather than aborting the remaining deletions, so a
 * partial failure reports every gateway's result (WEB-UIEX-08).
 */
export async function deleteGatewaysBounded(
  targets: readonly GatewayBulkDeleteTarget[],
  removeGateway: (gatewayId: string) => Promise<void>,
  limit: number = bulkDeleteConcurrencyLimit,
): Promise<GatewayBulkDeleteOutcome[]> {
  const outcomes = new Array<GatewayBulkDeleteOutcome>(targets.length);
  let cursor = 0;

  const worker = async (): Promise<void> => {
    for (;;) {
      // Reading and advancing the cursor is atomic on the single JS thread
      // because no await separates the read from the increment.
      const index = cursor;
      cursor += 1;
      if (index >= targets.length) {
        return;
      }
      const target = targets[index];
      if (!target) {
        return;
      }
      try {
        await removeGateway(target.id);
        outcomes[index] = {
          id: target.id,
          name: target.name,
          status: "accepted",
        };
      } catch (error) {
        outcomes[index] = {
          error,
          id: target.id,
          name: target.name,
          status: "failed",
        };
      }
    }
  };

  const workerCount = Math.min(Math.max(limit, 1), targets.length);
  await Promise.all(Array.from({ length: workerCount }, () => worker()));
  return outcomes;
}

type GatewayBulkDeletePhase = "confirming" | "deleting" | "idle" | "results";

export interface GatewayBulkDeleteController {
  cancel: () => void;
  confirm: () => void;
  dismissResults: () => void;
  isBusy: boolean;
  // Whether the confirmation dialog should be shown. True while awaiting
  // confirmation and while the confirmed deletion runs, but not during a retry,
  // which re-runs only the failures without reopening the dialog.
  isConfirmOpen: boolean;
  outcomes: readonly GatewayBulkDeleteOutcome[];
  phase: GatewayBulkDeletePhase;
  request: (targets: readonly GatewayBulkDeleteTarget[]) => void;
  retryFailed: () => void;
  targets: readonly GatewayBulkDeleteTarget[];
}

/**
 * Orchestrates the bulk-delete workflow: confirmation, bounded concurrent
 * execution that reuses the per-gateway deletion workflow, authoritative list
 * refresh, and partial-failure reporting with retry of only the failures
 * (WEB-UIEX-08). `onAccepted` receives the ids whose deletion the API accepted
 * so the caller can drop them from the selection while retaining the failures.
 */
export function useGatewayBulkDelete({
  onAccepted,
}: {
  onAccepted: (acceptedIds: readonly string[]) => void;
}): GatewayBulkDeleteController {
  const { gateways } = useGatewayUi();
  const queryClient = useQueryClient();
  const [phase, setPhase] = useState<GatewayBulkDeletePhase>("idle");
  const [targets, setTargets] = useState<readonly GatewayBulkDeleteTarget[]>(
    [],
  );
  const [outcomes, setOutcomes] = useState<readonly GatewayBulkDeleteOutcome[]>(
    [],
  );
  // Whether the in-flight deletion originated from the confirmation dialog, so
  // the dialog stays open to show progress during a confirmed delete but a retry
  // (which needs no reconfirmation) runs without reopening it.
  const [isDeletingFromConfirm, setIsDeletingFromConfirm] = useState(false);
  // Prevents a second run while one is in flight so a double submission cannot
  // resubmit requests that are already executing (WEB-UIEX-08.08).
  const runningRef = useRef(false);

  const run = useCallback(
    async (runTargets: readonly GatewayBulkDeleteTarget[]) => {
      if (runningRef.current || runTargets.length === 0) {
        return;
      }
      runningRef.current = true;
      setPhase("deleting");
      try {
        const results = await deleteGatewaysBounded(runTargets, (gatewayId) =>
          gateways.removeGateway(gatewayId),
        );
        const acceptedIds = results
          .filter((result) => result.status === "accepted")
          .map((result) => result.id);
        // Drop accepted gateways from the selection but keep the failures so the
        // user can retry them (WEB-UIEX-08.07).
        onAccepted(acceptedIds);
        // Drop the accepted gateways' detail caches, matching the per-row delete
        // workflow so a stale detail view cannot linger after bulk deletion.
        for (const acceptedId of acceptedIds) {
          queryClient.removeQueries({
            exact: true,
            queryKey: gatewayQueryKey(acceptedId),
          });
        }
        // Refetch the authoritative list and totals so pagination stays valid and
        // the selection reconciles against the refreshed rows (WEB-UIEX-08.06).
        await queryClient.invalidateQueries({ queryKey: gatewayListQueryRoot });
        setOutcomes(results);
        setPhase("results");
      } finally {
        runningRef.current = false;
      }
    },
    [gateways, onAccepted, queryClient],
  );

  return {
    cancel: () => {
      setPhase("idle");
      setTargets([]);
      setIsDeletingFromConfirm(false);
    },
    confirm: () => {
      setIsDeletingFromConfirm(true);
      void run(targets);
    },
    dismissResults: () => {
      setPhase("idle");
      setTargets([]);
      setOutcomes([]);
      setIsDeletingFromConfirm(false);
    },
    isBusy: phase === "deleting",
    isConfirmOpen:
      phase === "confirming" || (phase === "deleting" && isDeletingFromConfirm),
    outcomes,
    phase,
    request: (nextTargets) => {
      setTargets(nextTargets);
      setOutcomes([]);
      setIsDeletingFromConfirm(false);
      setPhase("confirming");
    },
    retryFailed: () => {
      const failed = outcomes
        .filter((outcome) => outcome.status === "failed")
        .map(({ id, name }) => ({ id, name }));
      setTargets(failed);
      setIsDeletingFromConfirm(false);
      void run(failed);
    },
    targets,
  };
}

/**
 * A gateway row's selection checkbox. Its accessible name identifies the gateway
 * (WEB-UIEX-08). An ineligible row keeps a visible checkbox but disables it with
 * an accessible explanation; selection never bypasses the server authorization
 * boundary.
 */
export function GatewaySelectionCheckbox({
  gateway,
  isSelected,
  onToggle,
}: {
  gateway: GatewayConnection;
  isSelected: boolean;
  onToggle: (gatewayId: string, selected: boolean) => void;
}) {
  const intl = useIntl();
  const deletable = isGatewayDeletable(gateway);
  // The accessible name carries the "why disabled" explanation for an ineligible
  // row, so assistive technology surfaces it even though a disabled control is
  // out of the tab order and cannot reveal the hover tooltip (WEB-UIEX-08.04).
  const checkbox = (
    <Checkbox
      aria-label={intl.formatMessage(
        deletable
          ? messages.selectGatewayRow
          : messages.selectGatewayRowDisabled,
        { gatewayName: gateway.name },
      )}
      id={`select-gateway-${gateway.id}`}
      isChecked={deletable ? isSelected : false}
      isDisabled={!deletable}
      onChange={(_event, checked) => {
        onToggle(gateway.id, checked);
      }}
    />
  );

  if (deletable) {
    return checkbox;
  }

  return (
    <Tooltip content={intl.formatMessage(messages.deleteGatewayNotPermitted)}>
      <span>{checkbox}</span>
    </Tooltip>
  );
}

/**
 * Header bulk-selection control (WEB-UIEX-08). A PatternFly
 * `MenuToggleCheckbox` shows the unchecked, checked, and indeterminate states
 * and the selected count; its menu scopes selection to the eligible rows on the
 * current page only.
 */
export function GatewayBulkSelectControl({
  eligibleCount,
  onSelectNone,
  onSelectPage,
  selectedCount,
}: {
  eligibleCount: number;
  onSelectNone: () => void;
  onSelectPage: () => void;
  selectedCount: number;
}) {
  const intl = useIntl();
  const [isOpen, setIsOpen] = useState(false);
  const checkboxId = useId();

  const isChecked: boolean | null =
    selectedCount === 0 ? false : selectedCount >= eligibleCount ? true : null;

  return (
    <Dropdown
      isOpen={isOpen}
      onOpenChange={setIsOpen}
      onSelect={() => {
        setIsOpen(false);
      }}
      toggle={(toggleRef) => (
        <MenuToggle
          isExpanded={isOpen}
          onClick={() => {
            setIsOpen((open) => !open);
          }}
          ref={toggleRef}
          splitButtonItems={[
            <MenuToggleCheckbox
              aria-label={intl.formatMessage(messages.bulkSelectAriaLabel)}
              id={`bulk-select-${checkboxId}`}
              isChecked={isChecked}
              key="bulk-select-checkbox"
              onChange={(checked) => {
                if (checked) {
                  onSelectPage();
                } else {
                  onSelectNone();
                }
              }}
            >
              {selectedCount > 0
                ? intl.formatMessage(messages.bulkSelectSelectedCount, {
                    count: selectedCount,
                  })
                : undefined}
            </MenuToggleCheckbox>,
          ]}
        />
      )}
    >
      <DropdownList>
        <DropdownItem
          key="none"
          onClick={() => {
            onSelectNone();
          }}
        >
          {intl.formatMessage(messages.bulkSelectNone, { count: 0 })}
        </DropdownItem>
        <DropdownItem
          key="page"
          onClick={() => {
            onSelectPage();
          }}
        >
          {intl.formatMessage(messages.bulkSelectPage, {
            count: eligibleCount,
          })}
        </DropdownItem>
      </DropdownList>
    </Dropdown>
  );
}

/**
 * Confirmation dialog shown before a bulk deletion (WEB-UIEX-08). It states how
 * many gateways will be deleted, identifies them, and explains the deletion
 * consequences. Cancel leaves every gateway unchanged.
 */
export function GatewayBulkDeleteConfirmModal({
  isBusy,
  onCancel,
  onConfirm,
  targets,
}: {
  isBusy: boolean;
  onCancel: () => void;
  onConfirm: () => void;
  targets: readonly GatewayBulkDeleteTarget[];
}) {
  const intl = useIntl();
  const descriptionId = useId();
  const titleId = useId();
  // Match the per-row delete's confirmation discipline: warn when any selected
  // gateway still has active sandboxes that deletion will disrupt.
  const gatewaysWithActiveSandboxes = targets.filter(
    (target) => (target.activeSandboxCount ?? 0) > 0,
  ).length;

  return (
    <Modal
      aria-describedby={descriptionId}
      aria-labelledby={titleId}
      isOpen
      onClose={isBusy ? undefined : onCancel}
      variant={ModalVariant.small}
    >
      <ModalHeader
        labelId={titleId}
        title={intl.formatMessage(messages.bulkDeleteConfirmTitle, {
          count: targets.length,
        })}
        titleIconVariant="warning"
      />
      <ModalBody>
        <Content id={descriptionId}>
          <FormattedMessage {...messages.bulkDeleteConfirmBody} />
        </Content>
        <List>
          {targets.map((target) => (
            <ListItem key={target.id}>{target.name}</ListItem>
          ))}
        </List>
        {gatewaysWithActiveSandboxes > 0 ? (
          <Alert
            isInline
            title={intl.formatMessage(messages.bulkDeleteActiveSandboxWarning, {
              gatewayCount: gatewaysWithActiveSandboxes,
            })}
            variant="warning"
          />
        ) : null}
      </ModalBody>
      <ModalFooter>
        {/* Keep the confirm button focusable while deleting (aria-disabled, not
            disabled) so the modal's focus trap always has a tabbable node; the
            controller's in-flight guard still blocks duplicate submissions. */}
        <Button
          isAriaDisabled={isBusy}
          isLoading={isBusy}
          onClick={onConfirm}
          spinnerAriaValueText={intl.formatMessage(messages.deletingGateway)}
          variant="danger"
        >
          {intl.formatMessage(messages.bulkDeleteConfirmButton, {
            count: targets.length,
          })}
        </Button>
        <Button isDisabled={isBusy} onClick={onCancel} variant="link">
          <FormattedMessage {...messages.cancel} />
        </Button>
      </ModalFooter>
    </Modal>
  );
}

/**
 * Per-gateway results banner for a completed bulk deletion (WEB-UIEX-08). It
 * distinguishes accepted asynchronous deletions from completed teardown, keeps
 * failed deletions visible for retry, and offers a retry of only the failures.
 */
export function GatewayBulkDeleteResultBanner({
  onDismiss,
  onRetry,
  outcomes,
}: {
  onDismiss: () => void;
  onRetry: () => void;
  outcomes: readonly GatewayBulkDeleteOutcome[];
}) {
  const intl = useIntl();
  // Move focus to the results region when it appears so keyboard and
  // screen-reader users land on the outcome summary, which also covers the case
  // where an all-success delete unmounts the triggering toolbar button.
  const regionRef = useRef<HTMLDivElement>(null);
  useEffect(() => {
    regionRef.current?.focus();
  }, []);
  const failed = outcomes.filter((outcome) => outcome.status === "failed");
  const hasFailures = failed.length > 0;
  const allFailed = failed.length === outcomes.length;

  const title = !hasFailures
    ? intl.formatMessage(messages.bulkDeleteResultTitleAllAccepted, {
        count: outcomes.length,
      })
    : allFailed
      ? intl.formatMessage(messages.bulkDeleteResultTitleAllFailed, {
          count: failed.length,
        })
      : intl.formatMessage(messages.bulkDeleteResultTitlePartial, {
          failed: failed.length,
          total: outcomes.length,
        });

  return (
    // A live, focusable region so the completion summary is announced to
    // assistive technology (standards/ui/accessibility.spec.md).
    <div aria-live="assertive" ref={regionRef} tabIndex={-1}>
      <Alert
        actionClose={
          <AlertActionCloseButton
            aria-label={intl.formatMessage(messages.close)}
            onClose={onDismiss}
          />
        }
        actionLinks={
          hasFailures ? (
            <AlertActionLink onClick={onRetry}>
              {intl.formatMessage(messages.bulkDeleteRetry)}
            </AlertActionLink>
          ) : undefined
        }
        isInline
        title={title}
        variant={!hasFailures ? "success" : allFailed ? "danger" : "warning"}
      >
        <List>
          {outcomes.map((outcome) => (
            <ListItem key={outcome.id}>
              {outcome.status === "accepted"
                ? intl.formatMessage(messages.bulkDeleteResultAccepted, {
                    gatewayName: outcome.name,
                  })
                : intl.formatMessage(messages.bulkDeleteResultFailed, {
                    gatewayName: outcome.name,
                  })}
            </ListItem>
          ))}
        </List>
      </Alert>
    </div>
  );
}
