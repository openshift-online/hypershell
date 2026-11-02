import {
  Alert,
  AlertActionLink,
  Button,
  Checkbox,
  Content,
  Dropdown,
  DropdownItem,
  DropdownList,
  EmptyState,
  EmptyStateBody,
  List,
  ListItem,
  MenuToggle,
  MenuToggleCheckbox,
  Modal,
  ModalBody,
  ModalFooter,
  ModalHeader,
  ModalVariant,
  Pagination,
  SearchInput,
  Switch,
  Toolbar,
  ToolbarContent,
  ToolbarGroup,
  ToolbarItem,
  Tooltip,
} from "@patternfly/react-core";
import { EllipsisVIcon } from "@patternfly/react-icons";
import { Table, Tbody, Td, Th, Thead, Tr } from "@patternfly/react-table";
import { useId, useState } from "react";

import { HealthStatus } from "../common/health-status";
import { ProvisionGatewayButton } from "../common/provision-gateway-button";
import { MockupShell } from "../shell/mockup-template";
import { GatewayListPageSurface } from "./gateway-list-page-surface";

export type GatewayListState = "loaded" | "empty" | "error";

// Design-review scenarios for row selection and the bulk-delete action
// (WEB-UIEX-08). Each story fixes one scenario so the selection, confirmation,
// and partial-failure states can be reviewed deterministically.
export type GatewayBulkScenario =
  | "none" // no selection; the bulk-delete action is disabled
  | "selection" // a subset of eligible rows selected (indeterminate header)
  | "confirm" // confirmation dialog open for the selected gateways
  | "partial-failure"; // results banner after a partial-failure bulk delete

type Gateway = {
  activeSandboxes: number;
  // Authoritative per-gateway delete capability from the API list response
  // (can_delete, WEB-UIEX-03/06). Selection eligibility is derived from this,
  // never from ownership or the ownership filter.
  canDelete: boolean;
  cluster: string;
  consoleUrl: string;
  created: string;
  createdBy: string;
  endpoint: string;
  name: string;
  status: string;
};

// Ordered newest first (creation time descending). The signed-in user (kim)
// owns two gateways and can only view a third owned by dana, so the ineligible
// row shows a disabled selection checkbox and the "Show only my gateways"
// switch is useful and shown.
const gateways: Gateway[] = [
  {
    activeSandboxes: 0,
    canDelete: true,
    cluster: "production-central",
    consoleUrl: "https://console.prod.example.com",
    created: "Oct 28, 2026",
    createdBy: "kim",
    endpoint: "https://gw-openshell-4lelb22e872df3c5.gw.localhost:443",
    name: "kim-gateway",
    status: "Healthy",
  },
  {
    activeSandboxes: 0,
    canDelete: true,
    cluster: "development-east",
    consoleUrl: "https://console.dev.example.com",
    created: "Sep 29, 2026",
    createdBy: "kim",
    endpoint: "https://gw-openshell-87adafc506ea322b.gw.localhost:443",
    name: "dev-gateway",
    status: "Healthy",
  },
  {
    activeSandboxes: 2,
    canDelete: false,
    cluster: "production-central",
    consoleUrl: "https://console.shared.example.com",
    created: "Aug 12, 2026",
    createdBy: "dana",
    endpoint: "https://gw-openshell-5b1c9d2e4f6a7081.gw.localhost:443",
    name: "shared-gateway",
    status: "Healthy",
  },
];

// Fixed selection per design-review scenario. Only can_delete rows are ever
// selected; the viewer-only "shared-gateway" is never eligible.
function selectedNamesFor(scenario: GatewayBulkScenario): string[] {
  switch (scenario) {
    case "selection":
      return ["kim-gateway"];
    case "confirm":
      return ["kim-gateway", "dev-gateway"];
    case "partial-failure":
      // The accepted deletion ("dev-gateway") clears; the failed deletion
      // ("kim-gateway") stays selected so the user can retry it.
      return ["kim-gateway"];
    default:
      return [];
  }
}

function GatewayRowActions({
  consoleUrl,
  gatewayName,
}: {
  consoleUrl: string;
  gatewayName: string;
}) {
  const [isOpen, setIsOpen] = useState(false);

  return (
    <Dropdown
      isOpen={isOpen}
      onOpenChange={setIsOpen}
      onSelect={() => setIsOpen(false)}
      popperProps={{ position: "right" }}
      toggle={(toggleRef) => (
        <MenuToggle
          aria-label={`Actions for ${gatewayName}`}
          isExpanded={isOpen}
          onClick={() => setIsOpen((open) => !open)}
          ref={toggleRef}
          variant="plain"
        >
          <EllipsisVIcon />
        </MenuToggle>
      )}
    >
      <DropdownList>
        <DropdownItem isExternalLink rel="noreferrer" to={consoleUrl}>
          Open gateway console
        </DropdownItem>
        <DropdownItem>Copy CLI connection command</DropdownItem>
        <DropdownItem>Rename gateway</DropdownItem>
        <DropdownItem isDanger>Delete gateway</DropdownItem>
      </DropdownList>
    </Dropdown>
  );
}

// Toolbar bulk-selection control (WEB-UIEX-08). A PatternFly MenuToggleCheckbox
// shows the checked, unchecked, and indeterminate states and the selected count;
// its menu scopes selection to the eligible rows on the current page only.
function BulkSelectControl({
  eligibleCount,
  selectedCount,
}: {
  eligibleCount: number;
  selectedCount: number;
}) {
  const [isOpen, setIsOpen] = useState(false);
  const checkboxId = useId();

  const isChecked =
    selectedCount === 0 ? false : selectedCount >= eligibleCount ? true : null;

  return (
    <Dropdown
      isOpen={isOpen}
      onOpenChange={setIsOpen}
      onSelect={() => setIsOpen(false)}
      toggle={(toggleRef) => (
        <MenuToggle
          aria-label="Bulk selection options"
          isExpanded={isOpen}
          onClick={() => setIsOpen((open) => !open)}
          ref={toggleRef}
          splitButtonItems={[
            <MenuToggleCheckbox
              aria-label="Select gateways on this page"
              id={`bulk-select-${checkboxId}`}
              isChecked={isChecked}
              key="bulk-select-checkbox"
            >
              {selectedCount > 0 ? `${selectedCount} selected` : undefined}
            </MenuToggleCheckbox>,
          ]}
        />
      )}
    >
      <DropdownList>
        <DropdownItem key="none">Select none (0 items)</DropdownItem>
        <DropdownItem key="page">
          Select page ({eligibleCount} items)
        </DropdownItem>
      </DropdownList>
    </Dropdown>
  );
}

function GatewayToolbar({
  eligibleCount = 0,
  itemCount = 0,
  ownershipFilterOn = false,
  selectedCount = 0,
  showBulkControls = false,
  showOwnershipFilter = false,
}: {
  eligibleCount?: number;
  itemCount?: number;
  ownershipFilterOn?: boolean;
  selectedCount?: number;
  showBulkControls?: boolean;
  showOwnershipFilter?: boolean;
}) {
  // "Show only my gateways" is a persistent on/off list filter, so it is a
  // PatternFly Switch rather than a pressed-state toggle button (WEB-UIEX-07).
  const [onlyMine, setOnlyMine] = useState(ownershipFilterOn);

  return (
    <Toolbar>
      {/* Row 1: selection, search, any other filters, the bulk-delete action,
          the primary Provision action, and pagination share one intentionally
          aligned row (WEB-UIEX-08), following the console.redhat.com Clusters
          list layout. */}
      <ToolbarContent>
        {showBulkControls ? (
          <ToolbarItem>
            <BulkSelectControl
              eligibleCount={eligibleCount}
              selectedCount={selectedCount}
            />
          </ToolbarItem>
        ) : null}
        <ToolbarItem>
          <SearchInput placeholder="Filter by name, cluster, status, or endpoint" />
        </ToolbarItem>
        {showBulkControls ? (
          <ToolbarGroup>
            <ToolbarItem>
              <Button
                isDanger
                isDisabled={selectedCount === 0}
                variant="secondary"
              >
                Delete
              </Button>
            </ToolbarItem>
            {selectedCount > 0 ? (
              <ToolbarItem>
                <Button variant="link">Clear selection</Button>
              </ToolbarItem>
            ) : null}
          </ToolbarGroup>
        ) : null}
        <ToolbarItem>
          <ProvisionGatewayButton />
        </ToolbarItem>
        <ToolbarItem align={{ default: "alignEnd" }}>
          <Pagination
            itemCount={itemCount}
            itemsEnd={itemCount}
            itemsStart={itemCount ? 1 : 0}
            page={1}
            perPage={20}
            variant="top"
          />
        </ToolbarItem>
      </ToolbarContent>
      {/* Row 2: the ownership switch on its own row directly below the filters
          (WEB-UIEX-07), so it reads as a persistent setting separate from the
          primary controls. The whole row is omitted when the switch is hidden,
          so no empty toolbar row remains. */}
      {showOwnershipFilter ? (
        <ToolbarContent>
          <ToolbarItem>
            <Switch
              id="gateways-only-mine"
              isChecked={onlyMine}
              label="Show only my gateways"
              onChange={(_event, checked) => setOnlyMine(checked)}
            />
          </ToolbarItem>
        </ToolbarContent>
      ) : null}
    </Toolbar>
  );
}

// Confirmation dialog shown before a bulk delete (WEB-UIEX-08). It states how
// many gateways will be deleted, identifies them, and explains that teardown is
// asynchronous. Cancel leaves every gateway unchanged.
function BulkDeleteConfirmModal({ names }: { names: string[] }) {
  const titleId = useId();

  return (
    <Modal
      aria-labelledby={titleId}
      isOpen
      onClose={() => undefined}
      variant={ModalVariant.small}
    >
      <ModalHeader
        labelId={titleId}
        title={`Delete ${names.length} gateways?`}
        titleIconVariant="warning"
      />
      <ModalBody>
        <Content>
          Deleting a gateway removes it from the platform and starts
          asynchronous teardown of its workload, database, and routing. This
          cannot be undone. The following gateways will be deleted:
        </Content>
        <List>
          {names.map((name) => (
            <ListItem key={name}>{name}</ListItem>
          ))}
        </List>
      </ModalBody>
      <ModalFooter>
        <Button isDanger variant="primary">
          Delete {names.length} gateways
        </Button>
        <Button variant="link">Cancel</Button>
      </ModalFooter>
    </Modal>
  );
}

// Per-gateway results banner for a partial-failure bulk delete (WEB-UIEX-08).
// Accepted deletions are distinguished from completed teardown; failures stay
// selected and can be retried without resubmitting the accepted deletions.
function BulkDeleteResultBanner() {
  return (
    <Alert
      actionLinks={<AlertActionLink>Retry failed deletions</AlertActionLink>}
      title="1 of 2 gateway deletions failed"
      variant="warning"
    >
      <List>
        <ListItem>
          dev-gateway: deletion accepted; resource teardown in progress.
        </ListItem>
        <ListItem>
          kim-gateway: deletion failed (the request was rejected). This gateway
          stays selected so you can retry it.
        </ListItem>
      </List>
    </Alert>
  );
}

function GatewaySelectionCell({
  canDelete,
  gatewayName,
  isSelected,
}: {
  canDelete: boolean;
  gatewayName: string;
  isSelected: boolean;
}) {
  const checkbox = (
    <Checkbox
      aria-label={`Select gateway ${gatewayName}`}
      id={`select-${gatewayName}`}
      isChecked={canDelete ? isSelected : false}
      isDisabled={!canDelete}
    />
  );

  if (canDelete) {
    return checkbox;
  }

  return (
    <Tooltip content="You can delete only gateways you own.">
      <span>{checkbox}</span>
    </Tooltip>
  );
}

function LoadedGatewayList({
  bulk = "none",
  ownershipFilterOn = false,
}: {
  bulk?: GatewayBulkScenario;
  ownershipFilterOn?: boolean;
}) {
  // Default order is newest first: the Created column sorted descending.
  const sortBy = { index: 5, direction: "desc" as const };
  const onSort = () => undefined;

  // When the switch is on, the API returns only the signed-in user's gateways
  // (owned by kim); the switch stays visible so the filter can be turned off.
  const visibleGateways = ownershipFilterOn
    ? gateways.filter((gateway) => gateway.createdBy === "kim")
    : gateways;

  const selectedNames = selectedNamesFor(bulk);
  const selectedSet = new Set(selectedNames);
  const eligibleCount = visibleGateways.filter(
    (gateway) => gateway.canDelete,
  ).length;

  return (
    <>
      <GatewayToolbar
        eligibleCount={eligibleCount}
        itemCount={visibleGateways.length}
        ownershipFilterOn={ownershipFilterOn}
        selectedCount={selectedNames.length}
        showBulkControls
        showOwnershipFilter
      />
      {bulk === "partial-failure" ? <BulkDeleteResultBanner /> : null}
      <Table aria-label="OpenShell Gateways">
        <Thead>
          <Tr>
            <Th screenReaderText="Row selection" />
            <Th sort={{ columnIndex: 0, onSort, sortBy: {} }}>Gateway name</Th>
            <Th sort={{ columnIndex: 1, onSort, sortBy: {} }}>
              Active sandboxes
            </Th>
            <Th sort={{ columnIndex: 2, onSort, sortBy: {} }}>Cluster</Th>
            <Th sort={{ columnIndex: 3, onSort, sortBy: {} }}>Status</Th>
            <Th sort={{ columnIndex: 4, onSort, sortBy }}>Created</Th>
            <Th sort={{ columnIndex: 5, onSort, sortBy: {} }}>Created by</Th>
            <Th sort={{ columnIndex: 6, onSort, sortBy: {} }}>
              Gateway endpoint
            </Th>
            <Th screenReaderText="Actions" />
          </Tr>
        </Thead>
        <Tbody>
          {visibleGateways.map((gateway) => (
            <Tr key={gateway.name}>
              <Td dataLabel="Row selection">
                <GatewaySelectionCell
                  canDelete={gateway.canDelete}
                  gatewayName={gateway.name}
                  isSelected={selectedSet.has(gateway.name)}
                />
              </Td>
              <Td dataLabel="Name">
                <Button
                  component="a"
                  href={`/gateways/${gateway.name}`}
                  variant="link"
                >
                  {gateway.name}
                </Button>
              </Td>
              <Td dataLabel="Active sandboxes">{gateway.activeSandboxes}</Td>
              <Td dataLabel="Cluster">{gateway.cluster}</Td>
              <Td dataLabel="Status">
                <HealthStatus label={gateway.status} />
              </Td>
              <Td dataLabel="Created">{gateway.created}</Td>
              <Td dataLabel="Created by">{gateway.createdBy}</Td>
              <Td dataLabel="Gateway endpoint">{gateway.endpoint}</Td>
              <Td isActionCell modifier="fitContent">
                <GatewayRowActions
                  consoleUrl={gateway.consoleUrl}
                  gatewayName={gateway.name}
                />
              </Td>
            </Tr>
          ))}
        </Tbody>
      </Table>
      {bulk === "confirm" ? (
        <BulkDeleteConfirmModal names={selectedNames} />
      ) : null}
    </>
  );
}

function EmptyGatewayList() {
  return (
    <>
      <GatewayToolbar />
      <EmptyState headingLevel="h2" titleText="No gateways">
        <EmptyStateBody>Provisioned gateways will appear here.</EmptyStateBody>
      </EmptyState>
    </>
  );
}

function ErrorGatewayList() {
  return (
    <>
      <GatewayToolbar />
      <Alert title="Unable to load gateways" variant="danger">
        The gateway list could not be loaded. Try refreshing the page.
      </Alert>
    </>
  );
}

export function GatewayListContent({
  bulk = "none",
  ownershipFilterOn = false,
  state,
}: {
  bulk?: GatewayBulkScenario;
  ownershipFilterOn?: boolean;
  state: GatewayListState;
}) {
  return (
    <>
      {state === "loaded" ? (
        <LoadedGatewayList bulk={bulk} ownershipFilterOn={ownershipFilterOn} />
      ) : null}
      {state === "empty" ? <EmptyGatewayList /> : null}
      {state === "error" ? <ErrorGatewayList /> : null}
    </>
  );
}

export function GatewayListMockup({
  bulk = "none",
  ownershipFilterOn = false,
  state,
}: {
  bulk?: GatewayBulkScenario;
  ownershipFilterOn?: boolean;
  state: GatewayListState;
}) {
  return (
    <MockupShell>
      <GatewayListPageSurface
        bulk={bulk}
        ownershipFilterOn={ownershipFilterOn}
        state={state}
      />
    </MockupShell>
  );
}
