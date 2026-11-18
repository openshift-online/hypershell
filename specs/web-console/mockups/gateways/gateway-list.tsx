import {
  Alert,
  AlertActionLink,
  Button,
  Checkbox,
  Dropdown,
  DropdownItem,
  DropdownList,
  EmptyState,
  EmptyStateBody,
  EmptyStateVariant,
  Content,
  List,
  ListItem,
  MenuToggle,
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

import { ProvisionGatewayButton } from "../common/provision-gateway-button";
import { MockupShell } from "../shell/mockup-template";
import { GatewayBulkSelectControl } from "../../../../packages/gateway-management-ui/src/gateways/gateway-bulk-delete";
import { GatewayConnectionCommandModal } from "../../../../packages/gateway-management-ui/src/gateways/gateway-connection-command-modal";
import { GatewayStatus } from "../../../../packages/gateway-management-ui/src/gateways/gateway-status";
import type { GatewayConnection } from "../../../../packages/gateway-management-ui/src/gateways/gateway-connections";
import { GatewayListPageSurface } from "./gateway-list-page-surface";

export type GatewayListState = "loaded" | "empty" | "error";

export type GatewayBulkScenario =
  "none" | "selection" | "confirm" | "partial-failure";

type Gateway = GatewayConnection & {
  created: string;
};

const gateways: Gateway[] = [
  {
    activeSandboxCount: 3,
    canDelete: true,
    clusterId: "cluster-ibm-public",
    clusterName: "Cluster East",
    clusterType: "IBM Public",
    consoleUrl: "https://console.dev.example.com",
    created: "Oct 29, 2026, 9:41 AM",
    createdBy: "kim-developer",
    endpoint: "https://gw-openshell-87adafc506ea322b.gw.localhost:443",
    gatewayVersion: "1.8.0",
    id: "payments-gateway",
    name: "payments-gateway",
    phase: "Running",
    status: "Healthy",
  },
  {
    activeSandboxCount: 1,
    canDelete: true,
    clusterId: "cluster-aws-vpn",
    clusterName: "Cluster North",
    clusterType: "AWS VPN",
    consoleUrl: "https://console.prod.example.com",
    created: "Oct 28, 2026, 4:12 PM",
    createdBy: "alex-operator",
    endpoint: "https://gw-openshell-4lelb22e872df3c5.gw.localhost:443",
    gatewayVersion: "1.8.0",
    id: "research-gateway",
    name: "research-gateway",
    phase: "Running",
    status: "Healthy",
  },
  {
    activeSandboxCount: 0,
    canDelete: false,
    clusterId: "cluster-ibm-public-kata",
    clusterName: "Cluster South",
    clusterType: "IBM Public Kata",
    consoleUrl: "https://console.staging.example.com",
    created: "Sep 22, 2026, 11:03 AM",
    createdBy: "casey-platform",
    endpoint: "https://gw-openshell-9f4b40e1c471c047.gw.localhost:443",
    gatewayVersion: "1.7.4",
    id: "integrations-gateway",
    name: "integrations-gateway",
    phase: "Degraded",
    status: "Attention required",
  },
];

function selectedNamesFor(scenario: GatewayBulkScenario): string[] {
  switch (scenario) {
    case "selection":
      return ["payments-gateway"];
    case "confirm":
      return ["payments-gateway", "research-gateway"];
    case "partial-failure":
      return ["payments-gateway"];
    default:
      return [];
  }
}

function GatewayRowActions({
  gateway,
  initialConnectionOpen = false,
}: {
  gateway: Gateway;
  initialConnectionOpen?: boolean;
}) {
  const [isOpen, setIsOpen] = useState(false);
  const [isConnectionOpen, setIsConnectionOpen] = useState(
    initialConnectionOpen,
  );

  return (
    <>
      <Dropdown
        isOpen={isOpen}
        onOpenChange={setIsOpen}
        onSelect={() => setIsOpen(false)}
        popperProps={{ position: "right" }}
        shouldFocusToggleOnSelect
        toggle={(toggleRef) => (
          <MenuToggle
            aria-label={`Actions for ${gateway.name}`}
            isExpanded={isOpen}
            onClick={() => setIsOpen((open) => !open)}
            ref={toggleRef}
            variant="plain"
          >
            <EllipsisVIcon aria-hidden="true" />
          </MenuToggle>
        )}
      >
        <DropdownList>
          <DropdownItem isExternalLink rel="noreferrer" to={gateway.consoleUrl}>
            Open gateway console
          </DropdownItem>
          <DropdownItem onClick={() => setIsConnectionOpen(true)}>
            Connection command
          </DropdownItem>
          <DropdownItem
            onClick={() => {
              if (gateway.endpoint) {
                void navigator.clipboard.writeText(gateway.endpoint);
              }
            }}
          >
            Copy endpoint
          </DropdownItem>
          <DropdownItem isAriaDisabled={gateway.canEdit === false}>
            Rename gateway
          </DropdownItem>
          <DropdownItem
            isAriaDisabled={gateway.canDelete === false}
            isDanger={gateway.canDelete !== false}
          >
            Delete gateway
          </DropdownItem>
        </DropdownList>
      </Dropdown>
      <GatewayConnectionCommandModal
        gateway={gateway}
        isOpen={isConnectionOpen}
        onClose={() => setIsConnectionOpen(false)}
      />
    </>
  );
}

function GatewayToolbar({
  eligibleCount = 0,
  isPlatformAdmin = false,
  itemCount = 0,
  ownershipFilterOn = false,
  selectedCount = 0,
  showBulkControls = false,
  showOwnershipFilter = false,
}: {
  eligibleCount?: number;
  isPlatformAdmin?: boolean;
  itemCount?: number;
  ownershipFilterOn?: boolean;
  selectedCount?: number;
  showBulkControls?: boolean;
  showOwnershipFilter?: boolean;
}) {
  const [onlyMine, setOnlyMine] = useState(ownershipFilterOn);

  return (
    <Toolbar>
      <ToolbarContent rowWrap={{ default: "wrap", md: "nowrap" }}>
        {showBulkControls ? (
          <ToolbarItem>
            <GatewayBulkSelectControl
              eligibleCount={eligibleCount}
              onSelectNone={() => undefined}
              onSelectPage={() => undefined}
              selectedCount={selectedCount}
            />
          </ToolbarItem>
        ) : null}
        <ToolbarItem>
          <SearchInput
            placeholder={
              isPlatformAdmin
                ? "Filter by name, status, endpoint, or created by"
                : "Filter by name, status, or endpoint"
            }
            style={{ width: "22rem" }}
          />
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
      </ToolbarContent>
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
          <ToolbarItem
            align={{ default: "alignStart", md: "alignEnd" }}
            variant="pagination"
          >
            <GatewayPagination itemCount={itemCount} variant="top" />
          </ToolbarItem>
        </ToolbarContent>
      ) : null}
    </Toolbar>
  );
}

function GatewayPagination({
  itemCount,
  variant,
}: {
  itemCount: number;
  variant: "top" | "bottom";
}) {
  return (
    <Pagination
      isCompact
      itemCount={itemCount}
      itemsEnd={itemCount}
      itemsStart={itemCount ? 1 : 0}
      page={1}
      perPage={20}
      perPageOptions={[
        { title: "10", value: 10 },
        { title: "20", value: 20 },
        { title: "50", value: 50 },
        { title: "100", value: 100 },
      ]}
      variant={variant}
    />
  );
}

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

function BulkDeleteResultBanner() {
  return (
    <Alert
      actionLinks={<AlertActionLink>Retry failed deletions</AlertActionLink>}
      title="1 of 2 gateway deletions failed"
      variant="warning"
    >
      <List>
        <ListItem>
          research-gateway: deletion accepted; resource teardown in progress.
        </ListItem>
        <ListItem>
          payments-gateway: deletion failed (the request was rejected). This
          gateway stays selected so you can retry it.
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
  connectionModalGateway,
  isPlatformAdmin = false,
  ownershipFilterOn = false,
}: {
  bulk?: GatewayBulkScenario;
  connectionModalGateway?: string;
  isPlatformAdmin?: boolean;
  ownershipFilterOn?: boolean;
}) {
  const sortBy = { index: 5, direction: "desc" as const };
  const onSort = () => undefined;
  const visibleGateways = ownershipFilterOn
    ? gateways.filter((gateway) => gateway.createdBy === "kim-developer")
    : gateways;
  const selectedNames = selectedNamesFor(bulk);
  const selectedSet = new Set(selectedNames);
  const eligibleCount = visibleGateways.filter(
    (gateway) => gateway.canDelete !== false,
  ).length;

  return (
    <>
      <GatewayToolbar
        eligibleCount={eligibleCount}
        isPlatformAdmin={isPlatformAdmin}
        itemCount={visibleGateways.length}
        ownershipFilterOn={ownershipFilterOn}
        selectedCount={selectedNames.length}
        showBulkControls
        showOwnershipFilter
      />
      {bulk === "partial-failure" ? <BulkDeleteResultBanner /> : null}
      <Table aria-label="OpenShell Gateways" variant="compact">
        <Thead>
          <Tr>
            <Th modifier="fitContent" screenReaderText="Row selection" />
            <Th sort={{ columnIndex: 0, onSort, sortBy: {} }} width={25}>
              Gateway name
            </Th>
            <Th sort={{ columnIndex: 1, onSort, sortBy: {} }} width={15}>
              Sandboxes
            </Th>
            <Th sort={{ columnIndex: 2, onSort, sortBy: {} }} width={15}>
              Status
            </Th>
            <Th sort={{ columnIndex: 3, onSort, sortBy: {} }} width={10}>
              Version
            </Th>
            <Th sort={{ columnIndex: 4, onSort, sortBy }} width={20}>
              Created
            </Th>
            {isPlatformAdmin ? (
              <Th sort={{ columnIndex: 5, onSort, sortBy: {} }} width={15}>
                Created by
              </Th>
            ) : null}
            <Th screenReaderText="Actions" />
          </Tr>
        </Thead>
        <Tbody>
          {visibleGateways.map((gateway) => (
            <Tr key={gateway.name}>
              <Td dataLabel="Row selection" modifier="fitContent">
                <GatewaySelectionCell
                  canDelete={gateway.canDelete !== false}
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
              <Td dataLabel="Sandboxes">{gateway.activeSandboxCount ?? 0}</Td>
              <Td dataLabel="Status">
                <GatewayStatus status={gateway.status} />
              </Td>
              <Td dataLabel="Version">{gateway.gatewayVersion}</Td>
              <Td dataLabel="Created">{gateway.created}</Td>
              {isPlatformAdmin ? (
                <Td dataLabel="Created by">
                  {gateway.createdBy ?? "Not available"}
                </Td>
              ) : null}
              <Td isActionCell modifier="fitContent">
                <GatewayRowActions
                  gateway={gateway}
                  initialConnectionOpen={
                    connectionModalGateway === gateway.name
                  }
                />
              </Td>
            </Tr>
          ))}
        </Tbody>
      </Table>
      <div style={{ display: "flex", justifyContent: "flex-end" }}>
        <GatewayPagination
          itemCount={visibleGateways.length}
          variant="bottom"
        />
      </div>
      {bulk === "confirm" ? (
        <BulkDeleteConfirmModal names={selectedNames} />
      ) : null}
    </>
  );
}

function EmptyGatewayList() {
  return (
    <>
      <GatewayToolbar itemCount={0} showOwnershipFilter />
      <EmptyState
        headingLevel="h2"
        titleText="No gateways"
        variant={EmptyStateVariant.sm}
      >
        <EmptyStateBody>Provisioned gateways will appear here.</EmptyStateBody>
      </EmptyState>
    </>
  );
}

function ErrorGatewayList() {
  return (
    <>
      <Alert isInline title="Unable to load gateways" variant="danger">
        The gateway list could not be loaded. Try refreshing the page.
      </Alert>
    </>
  );
}

export function GatewayListContent({
  bulk = "none",
  isPlatformAdmin = false,
  state,
  connectionModalGateway,
  ownershipFilterOn = false,
}: {
  bulk?: GatewayBulkScenario;
  isPlatformAdmin?: boolean;
  state: GatewayListState;
  connectionModalGateway?: string;
  ownershipFilterOn?: boolean;
}) {
  return (
    <>
      {state === "loaded" ? (
        <LoadedGatewayList
          bulk={bulk}
          connectionModalGateway={connectionModalGateway}
          isPlatformAdmin={isPlatformAdmin}
          ownershipFilterOn={ownershipFilterOn}
        />
      ) : null}
      {state === "empty" ? <EmptyGatewayList /> : null}
      {state === "error" ? <ErrorGatewayList /> : null}
    </>
  );
}

export function GatewayListMockup({
  bulk = "none",
  isPlatformAdmin = false,
  state,
  connectionModalGateway,
  ownershipFilterOn = false,
}: {
  bulk?: GatewayBulkScenario;
  isPlatformAdmin?: boolean;
  state: GatewayListState;
  connectionModalGateway?: string;
  ownershipFilterOn?: boolean;
}) {
  return (
    <MockupShell activeSection="gateways">
      <GatewayListPageSurface
        bulk={bulk}
        connectionModalGateway={connectionModalGateway}
        isPlatformAdmin={isPlatformAdmin}
        ownershipFilterOn={ownershipFilterOn}
        state={state}
      />
    </MockupShell>
  );
}
