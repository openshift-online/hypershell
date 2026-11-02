import {
  Alert,
  Button,
  Dropdown,
  DropdownItem,
  DropdownList,
  EmptyState,
  EmptyStateBody,
  Pagination,
  SearchInput,
  Switch,
  Toolbar,
  ToolbarContent,
  ToolbarItem,
  MenuToggle,
} from "@patternfly/react-core";
import { EllipsisVIcon } from "@patternfly/react-icons";
import { Table, Tbody, Td, Th, Thead, Tr } from "@patternfly/react-table";
import { useState } from "react";

import { HealthStatus } from "../common/health-status";
import { ProvisionGatewayButton } from "../common/provision-gateway-button";
import { MockupShell } from "../shell/mockup-template";
import { GatewayListPageSurface } from "./gateway-list-page-surface";

export type GatewayListState = "loaded" | "empty" | "error";

// Ordered newest first (creation time descending); gateways owned by the
// signed-in user (kim) and by another user (admin) both appear, so the
// "Show only my gateways" switch is useful and is shown.
const gateways = [
  {
    activeSandboxes: 0,
    cluster: "production-central",
    created: "Oct 28, 2026",
    createdBy: "kim",
    consoleUrl: "https://console.prod.example.com",
    endpoint: "https://gw-openshell-4lelb22e872df3c5.gw.localhost:443",
    name: "kim-gateway",
    status: "Healthy",
  },
  {
    activeSandboxes: 0,
    cluster: "development-east",
    created: "Sep 29, 2026",
    createdBy: "admin",
    consoleUrl: "https://console.dev.example.com",
    endpoint: "https://gw-openshell-87adafc506ea322b.gw.localhost:443",
    name: "dev-gateway",
    status: "Healthy",
  },
];

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

function GatewayToolbar({
  itemCount = 0,
  showOwnershipFilter = false,
  ownershipFilterOn = false,
}: {
  itemCount?: number;
  showOwnershipFilter?: boolean;
  ownershipFilterOn?: boolean;
}) {
  // "Show only my gateways" is a persistent on/off list filter, so it is a
  // PatternFly Switch rather than a pressed-state toggle button (WEB-UIEX-07).
  const [onlyMine, setOnlyMine] = useState(ownershipFilterOn);

  return (
    <Toolbar>
      {/* Row 1: search, any other filters, and the primary Provision action,
          following the console.redhat.com Clusters list layout. */}
      <ToolbarContent>
        <ToolbarItem>
          <SearchInput placeholder="Filter by name, cluster, status, or endpoint" />
        </ToolbarItem>
        <ToolbarItem>
          <ProvisionGatewayButton />
        </ToolbarItem>
        <ToolbarItem align={{ default: "alignEnd" }}>
          <Pagination
            itemCount={itemCount}
            itemsEnd={itemCount}
            itemsStart={itemCount ? 1 : 0}
            page={1}
            perPage={2}
            variant="top"
          />
        </ToolbarItem>
      </ToolbarContent>
      {/* Row 2: the ownership switch on its own row directly below the filters
          (WEB-UIEX-07). The whole row is omitted when the switch is hidden, so
          no empty toolbar row remains. */}
      {showOwnershipFilter ? (
        <ToolbarContent>
          <ToolbarItem>
            <Switch
              id="gateways-only-mine"
              label="Show only my gateways"
              isChecked={onlyMine}
              onChange={(_event, checked) => setOnlyMine(checked)}
            />
          </ToolbarItem>
        </ToolbarContent>
      ) : null}
    </Toolbar>
  );
}

function LoadedGatewayList({
  ownershipFilterOn = false,
}: {
  ownershipFilterOn?: boolean;
}) {
  // Default order is newest first: the Created column sorted descending.
  const sortBy = { index: 4, direction: "desc" as const };
  const onSort = () => undefined;

  // When the switch is on, the API returns only the signed-in user's gateways
  // (owned by kim); the switch stays visible so the filter can be turned off.
  const visibleGateways = ownershipFilterOn
    ? gateways.filter((gateway) => gateway.createdBy === "kim")
    : gateways;

  return (
    <>
      <GatewayToolbar
        itemCount={visibleGateways.length}
        ownershipFilterOn={ownershipFilterOn}
        showOwnershipFilter
      />
      <Table aria-label="OpenShell Gateways">
        <Thead>
          <Tr>
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
  state,
  ownershipFilterOn = false,
}: {
  state: GatewayListState;
  ownershipFilterOn?: boolean;
}) {
  return (
    <>
      {state === "loaded" ? (
        <LoadedGatewayList ownershipFilterOn={ownershipFilterOn} />
      ) : null}
      {state === "empty" ? <EmptyGatewayList /> : null}
      {state === "error" ? <ErrorGatewayList /> : null}
    </>
  );
}

export function GatewayListMockup({
  state,
  ownershipFilterOn = false,
}: {
  state: GatewayListState;
  ownershipFilterOn?: boolean;
}) {
  return (
    <MockupShell>
      <GatewayListPageSurface
        ownershipFilterOn={ownershipFilterOn}
        state={state}
      />
    </MockupShell>
  );
}
