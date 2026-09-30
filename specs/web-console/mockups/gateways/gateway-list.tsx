import {
  Alert,
  Dropdown,
  DropdownItem,
  DropdownList,
  EmptyState,
  EmptyStateBody,
  Pagination,
  SearchInput,
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
import { MockupTemplate } from "../shell/mockup-template";

export type GatewayListState = "loaded" | "empty" | "error";

const gateways = [
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
  {
    activeSandboxes: 0,
    cluster: "production-central",
    created: "Sep 29, 2026",
    createdBy: "admin",
    consoleUrl: "https://console.prod.example.com",
    endpoint: "https://gw-openshell-4lelb22e872df3c5.gw.localhost:443",
    name: "kim",
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
        <DropdownItem
          isExternalLink
          rel="noreferrer"
          to={consoleUrl}
        >
          Open gateway console
        </DropdownItem>
        <DropdownItem>Copy CLI connection command</DropdownItem>
        <DropdownItem>Rename gateway</DropdownItem>
        <DropdownItem isDanger>Delete gateway</DropdownItem>
      </DropdownList>
    </Dropdown>
  );
}

function GatewayToolbar({ itemCount = 0 }: { itemCount?: number }) {
  return (
    <Toolbar>
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
    </Toolbar>
  );
}

function LoadedGatewayList() {
  const sortBy = { index: 0, direction: "asc" as const };
  const onSort = () => undefined;

  return (
    <>
      <GatewayToolbar itemCount={gateways.length} />
      <Table aria-label="OpenShell Gateways">
        <Thead>
          <Tr>
            <Th sort={{ columnIndex: 0, onSort, sortBy }}>Gateway name</Th>
            <Th sort={{ columnIndex: 1, onSort, sortBy: {} }}>
              Active sandboxes
            </Th>
            <Th sort={{ columnIndex: 2, onSort, sortBy: {} }}>
              Cluster
            </Th>
            <Th sort={{ columnIndex: 3, onSort, sortBy: {} }}>
              Status
            </Th>
            <Th sort={{ columnIndex: 4, onSort, sortBy: {} }}>
              Created
            </Th>
            <Th sort={{ columnIndex: 5, onSort, sortBy: {} }}>
              Created by
            </Th>
            <Th sort={{ columnIndex: 6, onSort, sortBy: {} }}>
              Gateway endpoint
            </Th>
            <Th screenReaderText="Actions" />
          </Tr>
        </Thead>
        <Tbody>
          {gateways.map((gateway) => (
            <Tr key={gateway.name}>
              <Td dataLabel="Name">
                <a href={`/gateways/${gateway.name}`}>{gateway.name}</a>
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

export function GatewayListMockup({ state }: { state: GatewayListState }) {
  return (
    <MockupTemplate
      onRefresh={() => undefined}
      showBreadcrumbs={false}
      showRefresh
      title="OpenShell Gateways"
    >
      {state === "loaded" ? <LoadedGatewayList /> : null}
      {state === "empty" ? <EmptyGatewayList /> : null}
      {state === "error" ? <ErrorGatewayList /> : null}
    </MockupTemplate>
  );
}
