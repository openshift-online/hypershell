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
  Content,
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
  Stack,
  StackItem,
  Switch,
  Toolbar,
  ToolbarContent,
  ToolbarGroup,
  ToolbarItem,
  Tooltip,
} from "@patternfly/react-core";
import { CodeIcon, CopyIcon, EllipsisVIcon } from "@patternfly/react-icons";
import { Table, Tbody, Td, Th, Thead, Tr } from "@patternfly/react-table";
import { useId, useState } from "react";

import { HealthStatus } from "../common/health-status";
import { ProvisionGatewayButton } from "../common/provision-gateway-button";
import { MockupShell } from "../shell/mockup-template";
import { CommandBlock } from "../../../../packages/gateway-management-ui/src/gateways/editable-command";
import { GatewayListPageSurface } from "./gateway-list-page-surface";
import styles from "./gateway-list.module.css";

export type GatewayListState = "loaded" | "empty" | "error";

export type GatewayBulkScenario =
  "none" | "selection" | "confirm" | "partial-failure";

type Gateway = {
  activeSandboxes: number;
  canDelete: boolean;
  clusterType: string;
  consoleUrl: string;
  created: string;
  createdBy: string;
  endpoint: string;
  name: string;
  status: string;
  version: string;
};

const gateways: Gateway[] = [
  {
    activeSandboxes: 3,
    canDelete: true,
    clusterType: "IBM Public",
    created: "Oct 29, 2026, 9:41 AM",
    createdBy: "kim-developer",
    consoleUrl: "https://console.dev.example.com",
    endpoint: "https://gw-openshell-87adafc506ea322b.gw.localhost:443",
    name: "payments-gateway",
    status: "Healthy",
    version: "1.8.0",
  },
  {
    activeSandboxes: 1,
    canDelete: true,
    clusterType: "AWS VPN",
    created: "Oct 28, 2026, 4:12 PM",
    createdBy: "alex-operator",
    consoleUrl: "https://console.prod.example.com",
    endpoint: "https://gw-openshell-4lelb22e872df3c5.gw.localhost:443",
    name: "research-gateway",
    status: "Healthy",
    version: "1.8.0",
  },
  {
    activeSandboxes: 0,
    canDelete: false,
    clusterType: "IBM Public Kata",
    created: "Sep 22, 2026, 11:03 AM",
    createdBy: "casey-platform",
    consoleUrl: "https://console.staging.example.com",
    endpoint: "https://gw-openshell-9f4b40e1c471c047.gw.localhost:443",
    name: "integrations-gateway",
    status: "Attention required",
    version: "1.7.4",
  },
];

const connectionCommand = (gatewayName: string, endpoint: string) =>
  [
    "openshell gateway add \\",
    `  ${gatewayName} \\`,
    `  --endpoint ${endpoint}`,
  ].join("\n");

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
  const [onlyMine, setOnlyMine] = useState(ownershipFilterOn);

  return (
    <Toolbar>
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
          <SearchInput placeholder="Filter by name, status, or endpoint" />
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
          <GatewayPagination itemCount={itemCount} variant="top" />
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

function CopyValueButton({
  ariaLabel,
  icon,
  tooltip,
  value,
}: {
  ariaLabel: string;
  icon: React.ReactNode;
  tooltip: string;
  value: string;
}) {
  const [copied, setCopied] = useState(false);

  return (
    <Tooltip content={copied ? "Copied" : tooltip} position="top">
      <Button
        aria-label={ariaLabel}
        icon={icon}
        onClick={() => {
          void navigator.clipboard.writeText(value);
          setCopied(true);
          window.setTimeout(() => setCopied(false), 1500);
        }}
        variant="plain"
      />
    </Tooltip>
  );
}

function GatewayConnectionAction({
  endpoint,
  gatewayName,
  initialIsOpen = false,
}: {
  endpoint: string;
  gatewayName: string;
  initialIsOpen?: boolean;
}) {
  const [isOpen, setIsOpen] = useState(initialIsOpen);

  return (
    <>
      <Tooltip content={`View connection command for ${gatewayName}`}>
        <Button
          aria-label={`View connection command for ${gatewayName}`}
          icon={<CodeIcon />}
          onClick={() => setIsOpen(true)}
          variant="plain"
        />
      </Tooltip>
      {isOpen ? (
        <Modal
          isOpen
          onClose={() => setIsOpen(false)}
          variant={ModalVariant.medium}
        >
          <ModalHeader title={`Connect to ${gatewayName}`} />
          <ModalBody>
            <Stack hasGutter>
              <StackItem>
                <Content component="p">
                  Run this command in a terminal to add the gateway to the
                  OpenShell CLI.
                </Content>
              </StackItem>
              <StackItem>
                <div className={styles.steps}>
                  <CommandBlock
                    copyAriaLabel={`Copy connection command for ${gatewayName}`}
                    copyText={connectionCommand(gatewayName, endpoint)}
                  />
                </div>
              </StackItem>
            </Stack>
          </ModalBody>
          <ModalFooter>
            <Button onClick={() => setIsOpen(false)} variant="primary">
              Close
            </Button>
          </ModalFooter>
        </Modal>
      ) : null}
    </>
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
  ownershipFilterOn = false,
}: {
  bulk?: GatewayBulkScenario;
  connectionModalGateway?: string;
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
      <Table aria-label="OpenShell Gateways" variant="compact">
        <Thead>
          <Tr>
            <Th screenReaderText="Row selection" />
            <Th sort={{ columnIndex: 0, onSort, sortBy: {} }} width={15}>
              Gateway name
            </Th>
            <Th sort={{ columnIndex: 1, onSort, sortBy: {} }} width={10}>
              Sandboxes
            </Th>
            <Th sort={{ columnIndex: 2, onSort, sortBy: {} }} width={10}>
              Status
            </Th>
            <Th sort={{ columnIndex: 3, onSort, sortBy: {} }} width={10}>
              Version
            </Th>
            <Th sort={{ columnIndex: 4, onSort, sortBy: {} }} width={10}>
              Connect
            </Th>
            <Th sort={{ columnIndex: 5, onSort, sortBy }} width={15}>
              Created
            </Th>
            <Th sort={{ columnIndex: 6, onSort, sortBy: {} }} width={10}>
              Created by
            </Th>
            <Th width={10}>Cluster type</Th>
            <Th sort={{ columnIndex: 7, onSort, sortBy: {} }} width={10}>
              Endpoint
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
              <Td dataLabel="Sandboxes">{gateway.activeSandboxes}</Td>
              <Td dataLabel="Status">
                <HealthStatus
                  appearance={
                    gateway.status === "Attention required" ? "warning" : "good"
                  }
                  label={gateway.status}
                />
              </Td>
              <Td dataLabel="Version">{gateway.version}</Td>
              <Td dataLabel="Connect">
                <GatewayConnectionAction
                  endpoint={gateway.endpoint}
                  gatewayName={gateway.name}
                  initialIsOpen={connectionModalGateway === gateway.name}
                />
              </Td>
              <Td dataLabel="Created">{gateway.created}</Td>
              <Td dataLabel="Created by">{gateway.createdBy}</Td>
              <Td dataLabel="Cluster type">{gateway.clusterType}</Td>
              <Td dataLabel="Gateway endpoint">
                <CopyValueButton
                  ariaLabel={`Copy endpoint for ${gateway.name}`}
                  icon={<CopyIcon />}
                  tooltip={`Copy endpoint: ${gateway.endpoint}`}
                  value={gateway.endpoint}
                />
              </Td>
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
  state,
  connectionModalGateway,
  ownershipFilterOn = false,
}: {
  bulk?: GatewayBulkScenario;
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
  state,
  connectionModalGateway,
  ownershipFilterOn = false,
}: {
  bulk?: GatewayBulkScenario;
  state: GatewayListState;
  connectionModalGateway?: string;
  ownershipFilterOn?: boolean;
}) {
  return (
    <MockupShell activeSection="gateways">
      <GatewayListPageSurface
        bulk={bulk}
        connectionModalGateway={connectionModalGateway}
        ownershipFilterOn={ownershipFilterOn}
        state={state}
      />
    </MockupShell>
  );
}
