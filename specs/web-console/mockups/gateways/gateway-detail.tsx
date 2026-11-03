import {
  Accordion,
  AccordionContent,
  AccordionItem,
  AccordionToggle,
  ActionList,
  ActionListItem,
  Alert,
  Button,
  Card,
  CardBody,
  CardHeader,
  CardTitle,
  ClipboardCopy,
  Content,
  DescriptionList,
  DescriptionListDescription,
  DescriptionListGroup,
  DescriptionListTerm,
  Dropdown,
  DropdownItem,
  DropdownList,
  ExpandableSection,
  Flex,
  FlexItem,
  Form,
  FormGroup,
  Grid,
  GridItem,
  HelperText,
  HelperTextItem,
  Label,
  MenuToggle,
  Modal,
  ModalBody,
  ModalFooter,
  ModalHeader,
  ModalVariant,
  PageSection,
  Pagination,
  Radio,
  SearchInput,
  Select,
  SelectList,
  SelectOption,
  Stack,
  StackItem,
  Tab,
  Tabs,
  TabTitleText,
  TextArea,
  TextInput,
  TextInputGroup,
  TextInputGroupMain,
  Title,
  Toolbar,
  ToolbarContent,
  ToolbarGroup,
  ToolbarItem,
  Tooltip,
} from "@patternfly/react-core";
import {
  ExpandableRowContent,
  Table,
  Tbody,
  Td,
  Th,
  Thead,
  Tr,
} from "@patternfly/react-table";
import {
  CopyIcon,
  CodeIcon,
  EllipsisVIcon,
  ExternalLinkAltIcon,
  SearchIcon,
  SyncAltIcon,
} from "@patternfly/react-icons";
import { useState } from "react";
import { IntlProvider } from "react-intl";

import { HealthStatus } from "../common/health-status";
import { MockupShell } from "../shell/mockup-template";
import { GatewayProvisioningStepper } from "../../../../packages/gateway-management-ui/src/gateways/gateway-provisioning-stepper";
import { CommandBlock } from "../../../../packages/gateway-management-ui/src/gateways/editable-command";
import {
  buildGatewayAddCommand,
  buildOpenShellInstallCommand,
  sandboxConnectDocsUrl,
  shellArgument,
  type GatewayConnection,
} from "../../../../packages/gateway-management-ui/src/gateways/gateway-connections";
import { GatewayDetailPageSurface } from "./gateway-detail-page-surface";
import styles from "./gateway-detail.module.css";

export type GatewayDetailTab =
  "connection" | "manage-access" | "service-accounts" | "details";

export type GatewayDetailModal =
  | "rename-gateway"
  | "delete-gateway"
  | "add-users"
  | "workspace-access"
  | "change-own-role"
  | "remove-access"
  | "create-service-account"
  | "service-account-setup"
  | "edit-service-account"
  | "revoke-service-account"
  | "delete-service-account";

export type GatewayDetailAttention =
  | "gateway-health"
  | "sandbox-expiring"
  | "sandbox-idle"
  | "service-account-expiring";

const accessGrants = [
  {
    grantedAt: "Oct 12, 2026, 2:15 PM",
    name: "Kim Developer",
    role: "Owner",
    subject: "00000000-1111-2222-3333-444444444444",
    username: "kim-developer",
  },
  {
    grantedAt: "Oct 18, 2026, 11:05 AM",
    name: "Alex Operator",
    role: "Admin",
    subject: "11111111-2222-3333-4444-555555555555",
    username: "alex-operator",
  },
  {
    grantedAt: "Oct 28, 2026, 4:40 PM",
    name: "Jordan Automation",
    role: "User",
    subject: "22222222-3333-4444-5555-666666666666",
    username: "jordan-automation",
  },
];

const serviceAccounts = [
  {
    clientId: "hs-sa-1111-LLLL",
    createdAt: "Oct 28, 2026, 10:35 PM",
    description: null,
    expiration: "Jan 26, 2027, 9:35 PM",
    name: "ci-pipeline",
    role: "openshell-admin",
    expiresSoon: false,
    subject: "11111111-2222-3333-4444-555555555555",
    status: "Ready",
    workspaces: ["payments-platform"],
  },
  {
    clientId: "hs-sa-2222-MMMM",
    createdAt: "Oct 28, 2026, 9:12 PM",
    description: "Metrics and trace collection client",
    expiration: "Nov 3, 2026, 8:15 AM",
    name: "observability",
    role: "openshell-user",
    expiresSoon: true,
    subject: "11111111-2222-3333-4444-666666666666",
    status: "Provisioning",
    workspaces: ["observability", "payments-platform"],
  },
];

const workspaceMembershipCommand = (subject: string, workspace: string) =>
  [
    `WORKSPACE_NAME='${workspace}'`,
    "",
    "openshell workspace member add \\",
    '  --workspace "$WORKSPACE_NAME" \\',
    `  --subject ${subject} \\`,
    "  --role user",
  ].join("\n");

const serviceAccountJwtCommand = (clientId: string) =>
  `curl --request POST "https://sso.example.com/realms/openshell/protocol/openid-connect/token" --data "grant_type=client_credentials" --data "client_id=${clientId}" --data "client_secret=$CLIENT_SECRET"`;

const expiringServiceAccounts = serviceAccounts.filter(
  ({ expiresSoon }) => expiresSoon,
);

const mockServiceAccountSecret = "mock-one-time-client-secret";

const pageSizes = [
  { title: "10", value: 10 },
  { title: "20", value: 20 },
  { title: "50", value: 50 },
  { title: "100", value: 100 },
];

const mockGateway: GatewayConnection = {
  activeSandboxCount: 3,
  canDelete: true,
  canEdit: true,
  clusterId: "cluster-aws-us-east-1",
  clusterName: "aws-us-east-1-production",
  consoleUrl: "https://payments-gateway.example.com",
  createdAt: "2026-10-01T09:30:00Z",
  createdBy: "kim-developer",
  endpoint: "https://payments-gateway.example.com:443",
  gatewayVersion: "1.8.0",
  id: "gateway-payments",
  name: "payments-gateway",
  oidcAudience: "openshell-payments",
  oidcClientId: "payments-client",
  oidcIssuer: "https://sso.example.com/realms/openshell",
  phase: "Running",
  status: "Healthy",
};

const provisioningConditions = [
  { conditionStatus: "Complete", message: "", type: "EnvironmentReady" },
  { conditionStatus: "Complete", message: "", type: "DatabaseReady" },
  { conditionStatus: "Complete", message: "", type: "IdentityProviderReady" },
  { conditionStatus: "Complete", message: "", type: "GatewayDeployed" },
  { conditionStatus: "Complete", message: "", type: "GatewayHealthy" },
] as const;

const disallowedCommandValue = /[^A-Za-z0-9_./:@%+=,-]+/g;
const enmaasDocsUrl =
  "https://source.redhat.com/departments/products_and_global_engineering/workload_management/enmaas";
const enmaasProviderName = "my-enmaas";
const enmaasModel = "rits/zai-org/glm-5-3";
const enmaasSandboxName = "enmaas-claude";
const enmaasHostedImage =
  "quay.io/redhat-services-prod/hcm-eng-prod-tenant/hypershell-main/hypershell-agent-runtime-main@sha256:0d7f71e4c078f5e637e6ea03e5f53b317dc65d6c9765c7810f27dd111cca6b64";

function sanitizeCommandValue(value: string): string {
  return value.replace(disallowedCommandValue, "");
}

function buildEnmaasProfileCommand(): string {
  return [
    "cat > enmaas.yaml <<'YAML'",
    "id: enmaas",
    "display_name: EnMaaS",
    "description: Anthropic-compatible EnMaaS API",
    "category: inference",
    "inference_capable: true",
    "credentials:",
    "  - name: api_key",
    "    description: EnMaaS API key used by Claude Code",
    "    env_vars: [ANTHROPIC_API_KEY]",
    "    required: true",
    "    auth_style: header",
    "    header_name: x-api-key",
    "discovery:",
    "  credentials: [api_key]",
    "endpoints:",
    "  - host: api.enmaas.devshift.net",
    "    port: 443",
    "    protocol: rest",
    "    access: read-write",
    "    enforcement: enforce",
    "binaries:",
    "  - /usr/local/bin/claude",
    "YAML",
  ].join("\n");
}

function buildEnmaasProfileImportCommand(): string {
  return [
    "if ! openshell profile describe enmaas --global >/dev/null 2>&1; then",
    "  openshell profile lint -f enmaas.yaml",
    "  openshell profile import -f enmaas.yaml --global",
    "fi",
  ].join("\n");
}

function buildEnmaasProviderCreateCommand(
  gatewayName: string,
  providerName: string,
): string {
  return [
    `if ! openshell provider get -g ${shellArgument(gatewayName)} ${shellArgument(providerName)} >/dev/null 2>&1; then`,
    '  if [[ -z "${ENMAAS_KEY:-}" ]]; then',
    "    printf 'Enter ENMAAS_KEY: '",
    "    read -rs ENMAAS_KEY",
    "    printf '\\n'",
    "    export ENMAAS_KEY",
    "  fi",
    "",
    '  : "${ENMAAS_KEY:?ENMAAS_KEY must not be empty}"',
    "",
    '  ANTHROPIC_API_KEY="$ENMAAS_KEY" openshell provider create',
    `    -g ${shellArgument(gatewayName)}`,
    `    --name ${shellArgument(providerName)}`,
    "    --type enmaas",
    "    --global-profile",
    "    --from-existing",
    "fi",
  ].join("\n");
}

function buildOpenClaudeCommand(name: string): string {
  return `openshell sandbox connect ${shellArgument(name)}`;
}

function buildConnectionSandboxCommand(
  gatewayName: string,
  name: string,
  model: string,
  providerName: string,
): string {
  const settings = JSON.stringify({
    modelPicker: {
      options: [
        {
          model,
          label: "EnMaaS model",
          description: "Hosted via EnMaaS",
        },
      ],
    },
  });

  return [
    "openshell sandbox create",
    `  --name ${shellArgument(name)}`,
    `  --from ${shellArgument(enmaasHostedImage)}`,
    "  --cpu 500m",
    "  --memory 512Mi",
    `  --provider ${shellArgument(providerName)}`,
    `  -g ${shellArgument(gatewayName)}`,
    "  --env=ANTHROPIC_BASE_URL=https://api.enmaas.devshift.net",
    "  --env=CLAUDE_CODE_ENABLE_GATEWAY_MODEL_DISCOVERY=1",
    "  --no-auto-providers",
    `  -- bash -lc "claude --bare --settings ${shellArgument(settings)} --model ${shellArgument(model)}; exec bash -l"`,
  ].join("\n");
}

function ProductionConnectionTab({
  onChangeTab,
}: {
  onChangeTab: (tab: GatewayDetailTab) => void;
}) {
  const [expandedStep, setExpandedStep] = useState("install-cli");
  const [allCommandsCopied, setAllCommandsCopied] = useState(false);
  const [providerName, setProviderName] = useState(enmaasProviderName);
  const [modelName, setModelName] = useState(enmaasModel);
  const [sandboxName, setSandboxName] = useState(enmaasSandboxName);

  const installCommand = buildOpenShellInstallCommand(mockGateway) ?? "";
  const gatewayCommand = [
    `if ! openshell gateway list -o json | grep -q "\\"name\\": \\"${mockGateway.name}\\""; then`,
    `  ${buildGatewayAddCommand(mockGateway)?.replaceAll("\n", "\n  ") ?? ""}`,
    "fi",
    `openshell gateway select ${shellArgument(mockGateway.name)}`,
  ].join("\n");
  const profileCommand = buildEnmaasProfileCommand();
  const profileImportCommand = buildEnmaasProfileImportCommand();
  const providerCommand = buildEnmaasProviderCreateCommand(
    mockGateway.name,
    providerName,
  );
  const sandboxCommand = buildConnectionSandboxCommand(
    mockGateway.name,
    sandboxName,
    modelName,
    providerName,
  );
  const idempotentSandboxCommand = [
    `if ! openshell sandbox get -g ${shellArgument(mockGateway.name)} ${shellArgument(sandboxName)} >/dev/null 2>&1; then`,
    `  ${sandboxCommand.replaceAll("\n", "\n  ")}`,
    "fi",
  ].join("\n");
  const openClaudeCommand = buildOpenClaudeCommand(sandboxName);

  const connectionSteps = [
    {
      command: installCommand,
      description:
        "Only needed once. Installs the OpenShell CLI version that matches this gateway. Later steps require the OpenShell CLI.",
      id: "install-cli",
      title: "Install the OpenShell CLI",
      timing: "One-time",
    },
    {
      command: gatewayCommand,
      description:
        "Adds and authenticates the gateway if needed, then selects it. Existing gateway registrations are reused.",
      id: "add-gateway",
      title: "Add this gateway",
      timing: "One-time",
    },
    {
      command: profileCommand,
      description:
        "Creates the local provider profile that binds your EnMaaS key to the EnMaaS endpoint.",
      id: "create-profile",
      title: "Create the EnMaaS provider profile",
      timing: "One-time",
    },
    {
      command: profileImportCommand,
      description:
        "Validates and imports the EnMaaS provider profile if it is not already installed.",
      id: "import-profile",
      title: "Import the EnMaaS provider profile",
      timing: "One-time",
    },
    {
      command: providerCommand,
      description:
        "Creates the provider if needed. It prompts for your EnMaaS key only when creating a new provider.",
      id: "create-provider",
      title: "Connect this gateway to EnMaaS",
      timing: "One-time",
    },
    {
      command: idempotentSandboxCommand,
      description:
        "Creates the sandbox if needed. Exit Claude to explore the sandbox file system.",
      id: "create-sandbox",
      title: "Create a Claude sandbox",
      timing: "Every time",
    },
    {
      command: openClaudeCommand,
      description:
        "Attaches directly to the running Claude process in the sandbox.",
      id: "open-claude",
      title: "Open Claude in the sandbox",
      timing: "Every time",
    },
  ];

  const allCommands = connectionSteps
    .map(({ command }) => command)
    .join("\n\n");

  return (
    <Stack hasGutter>
      <StackItem>
        <Flex justifyContent={{ default: "justifyContentSpaceBetween" }}>
          <FlexItem>
            <Title headingLevel="h2" size="xl">
              Connection
            </Title>
            <Content component="p">
              Connect your terminal to this gateway and start a Claude sandbox
              attached to EnMaaS.
            </Content>
          </FlexItem>
          <FlexItem>
            <Button
              aria-label="Refresh connection details"
              icon={<SyncAltIcon />}
              title="Refresh connection details"
              variant="plain"
            />
          </FlexItem>
        </Flex>
      </StackItem>
      <StackItem>
        <Alert component="h3" isInline title="Before you begin" variant="info">
          <Content component="p">
            Export your EnMaaS key as ENMAAS_KEY in your terminal. The provider
            command will prompt for it if the variable is not set.
          </Content>
          <Button
            aria-label="Learn about EnMaaS and get a key (opens in a new tab)"
            component="a"
            href={enmaasDocsUrl}
            icon={<ExternalLinkAltIcon />}
            iconPosition="end"
            isInline
            rel="noopener noreferrer"
            size="sm"
            target="_blank"
            variant="link"
          >
            Learn about EnMaaS and get a key
          </Button>
        </Alert>
      </StackItem>
      <StackItem>
        <Flex
          justifyContent={{ default: "justifyContentFlexEnd" }}
          flexWrap={{ default: "wrap" }}
        >
          <FlexItem>
            <Button
              icon={<CopyIcon />}
              onClick={() => {
                void navigator.clipboard.writeText(allCommands);
                setAllCommandsCopied(true);
                window.setTimeout(() => {
                  setAllCommandsCopied(false);
                }, 1500);
              }}
              variant="secondary"
            >
              {allCommandsCopied ? "Copied" : "Copy all commands"}
            </Button>
          </FlexItem>
        </Flex>
      </StackItem>
      <StackItem>
        <Accordion className={styles.connectionGuide} isBordered>
          {connectionSteps.map(
            ({ command, description, id, title, timing }, index) => (
              <AccordionItem isExpanded={expandedStep === id} key={id}>
                <AccordionToggle
                  id={`${id}-toggle`}
                  onClick={() => {
                    setExpandedStep(expandedStep === id ? "" : id);
                  }}
                >
                  <span className={styles.connectionStepTitle}>
                    <span className={styles.connectionStepNumber}>
                      {index + 1}
                    </span>
                    <span>{title}</span>
                    {timing === "One-time" ? (
                      <Label color="grey" isCompact variant="outline">
                        One-time
                      </Label>
                    ) : null}
                  </span>
                </AccordionToggle>
                <AccordionContent id={`${id}-content`}>
                  <Stack hasGutter>
                    <StackItem>
                      <Content component="p">{description}</Content>
                    </StackItem>
                    {id === "open-editor" ? (
                      <StackItem>
                        <Button
                          aria-label="OpenShell connection documentation (opens in a new tab)"
                          component="a"
                          href={sandboxConnectDocsUrl}
                          icon={<ExternalLinkAltIcon />}
                          iconPosition="end"
                          isInline
                          rel="noopener noreferrer"
                          target="_blank"
                          variant="link"
                        >
                          OpenShell connection documentation
                        </Button>
                      </StackItem>
                    ) : null}
                    {id === "create-provider" ? (
                      <StackItem>
                        <Form className={styles.commandSettings}>
                          <Grid hasGutter>
                            <GridItem span={12} lg={4}>
                              <FormGroup
                                fieldId="connection-provider-name"
                                label="Provider name"
                              >
                                <TextInput
                                  id="connection-provider-name"
                                  onChange={(_event, value) => {
                                    setProviderName(
                                      sanitizeCommandValue(value),
                                    );
                                  }}
                                  value={providerName}
                                />
                              </FormGroup>
                            </GridItem>
                          </Grid>
                          <HelperText>
                            <HelperTextItem>
                              Also used when creating the sandbox.
                            </HelperTextItem>
                          </HelperText>
                        </Form>
                      </StackItem>
                    ) : null}
                    {id === "create-sandbox" ? (
                      <StackItem>
                        <Form className={styles.commandSettings}>
                          <Grid hasGutter>
                            <GridItem span={12} lg={6}>
                              <FormGroup
                                fieldId="connection-sandbox-name"
                                label="Sandbox name"
                              >
                                <TextInput
                                  id="connection-sandbox-name"
                                  onChange={(_event, value) => {
                                    setSandboxName(sanitizeCommandValue(value));
                                  }}
                                  value={sandboxName}
                                />
                              </FormGroup>
                            </GridItem>
                            <GridItem span={12} lg={6}>
                              <FormGroup
                                fieldId="connection-model-name"
                                label="Claude model"
                              >
                                <TextInput
                                  id="connection-model-name"
                                  onChange={(_event, value) => {
                                    setModelName(sanitizeCommandValue(value));
                                  }}
                                  value={modelName}
                                />
                              </FormGroup>
                            </GridItem>
                          </Grid>
                          <HelperText>
                            <HelperTextItem>
                              The sandbox name is also used when connecting your
                              editor.
                            </HelperTextItem>
                          </HelperText>
                        </Form>
                      </StackItem>
                    ) : null}
                    <StackItem>
                      <div className={styles.steps}>
                        <CommandBlock
                          copyAriaLabel={`Copy command for step ${index + 1}, ${title}`}
                          copyText={command}
                        />
                      </div>
                    </StackItem>
                  </Stack>
                </AccordionContent>
              </AccordionItem>
            ),
          )}
        </Accordion>
      </StackItem>
      <StackItem>
        <Card className={styles.automationCard}>
          <CardHeader>
            <CardTitle>
              <Title headingLevel="h3" size="lg">
                Automate this gateway
              </Title>
            </CardTitle>
          </CardHeader>
          <CardBody>
            <Content component="p">
              Use a service account when a CI/CD pipeline or bot needs access
              without a personal user account.
            </Content>
            <Button
              onClick={() => {
                onChangeTab("service-accounts");
              }}
              variant="link"
            >
              Manage service accounts
            </Button>
          </CardBody>
        </Card>
      </StackItem>
    </Stack>
  );
}

function MockPagination({
  itemCount,
  variant,
}: {
  itemCount: number;
  variant?: "top" | "bottom";
}) {
  return (
    <Pagination
      isCompact
      itemCount={itemCount}
      itemsEnd={itemCount}
      itemsStart={itemCount ? 1 : 0}
      page={1}
      perPage={20}
      perPageOptions={pageSizes}
      variant={variant ?? "top"}
    />
  );
}

function AccessTab({
  isProvisioning = false,
  onOpenServiceAccounts,
  onOpenWorkspaceAccess,
}: {
  isProvisioning?: boolean;
  onOpenServiceAccounts: () => void;
  onOpenWorkspaceAccess: (username: string) => void;
}) {
  const [sortColumn, setSortColumn] = useState("grantedAt");
  const [sortDirection, setSortDirection] = useState<"asc" | "desc">("desc");
  const columns = ["name", "username", "role", "grantedAt"];
  const activeSortIndex = Math.max(0, columns.indexOf(sortColumn));
  const getSortParams = (columnIndex: number) => ({
    columnIndex,
    onSort: (_event: unknown, _index: number, direction: "asc" | "desc") => {
      setSortColumn(columns[columnIndex] ?? "grantedAt");
      setSortDirection(direction);
    },
    sortBy: {
      direction: sortDirection,
      index: activeSortIndex,
    },
  });

  return (
    <Stack hasGutter>
      <StackItem>
        <Flex justifyContent={{ default: "justifyContentSpaceBetween" }}>
          <FlexItem>
            <Title headingLevel="h2" size="xl">
              User access
            </Title>
            <Content component="p">
              Manage which people can access this gateway and at what role.
            </Content>
          </FlexItem>
          <FlexItem>
            <Button
              aria-label="Refresh access list"
              icon={<SyncAltIcon />}
              title="Refresh access list"
              variant="plain"
            />
          </FlexItem>
        </Flex>
      </StackItem>
      {isProvisioning ? (
        <StackItem>
          <Alert isInline title="User access can be granted now" variant="info">
            People you add will be able to sign in once gateway provisioning
            completes.
          </Alert>
        </StackItem>
      ) : null}
      <StackItem>
        <Toolbar>
          <ToolbarContent>
            <ToolbarGroup variant="filter-group">
              <ToolbarItem>
                <SearchInput
                  aria-label="Filter by name or user ID"
                  className={styles.accessFilter}
                  placeholder="Filter by name or user ID"
                />
              </ToolbarItem>
              <ToolbarItem>
                <Select
                  isOpen={false}
                  onOpenChange={() => undefined}
                  selected="All roles"
                  toggle={(toggleRef) => (
                    <MenuToggle ref={toggleRef}>All roles</MenuToggle>
                  )}
                >
                  <SelectList>
                    <SelectOption value="all">All roles</SelectOption>
                  </SelectList>
                </Select>
              </ToolbarItem>
            </ToolbarGroup>
            <ToolbarGroup variant="action-group">
              <ToolbarItem>
                <Button variant="primary">Add user</Button>
              </ToolbarItem>
            </ToolbarGroup>
            <ToolbarItem variant="pagination">
              <MockPagination itemCount={accessGrants.length} variant="top" />
            </ToolbarItem>
          </ToolbarContent>
        </Toolbar>
      </StackItem>
      <StackItem>
        <Table aria-label="Gateway access" variant="compact">
          <Thead>
            <Tr>
              <Th sort={getSortParams(0)} width={25}>
                Name
              </Th>
              <Th sort={getSortParams(1)} width={25}>
                User ID
              </Th>
              <Th sort={getSortParams(2)} width={20}>
                Role
              </Th>
              <Th sort={getSortParams(3)} width={20}>
                Granted
              </Th>
              <Th screenReaderText="Actions" />
            </Tr>
          </Thead>
          <Tbody>
            {accessGrants.map((grant) => (
              <Tr key={grant.username}>
                <Td dataLabel="Name">{grant.name}</Td>
                <Td dataLabel="User ID">{grant.username}</Td>
                <Td dataLabel="Role">
                  <Flex
                    alignItems={{ default: "alignItemsCenter" }}
                    spaceItems={{ default: "spaceItemsSm" }}
                  >
                    <FlexItem>
                      {grant.role === "Owner" ? (
                        <Tooltip content="A gateway must keep at least one owner.">
                          <Button
                            aria-label="A gateway must keep at least one owner."
                            isAriaDisabled
                            variant="plain"
                          >
                            Owner
                          </Button>
                        </Tooltip>
                      ) : (
                        <MenuToggle variant="plainText">
                          {grant.role}
                        </MenuToggle>
                      )}
                    </FlexItem>
                    {grant.role === "User" ? (
                      <FlexItem>
                        <Tooltip content="View workspace access command">
                          <Button
                            aria-label={`View workspace access command for ${grant.name}`}
                            icon={<CodeIcon />}
                            onClick={() =>
                              onOpenWorkspaceAccess(grant.username)
                            }
                            variant="plain"
                          />
                        </Tooltip>
                      </FlexItem>
                    ) : null}
                  </Flex>
                </Td>
                <Td dataLabel="Granted">{grant.grantedAt}</Td>
                <Td isActionCell modifier="fitContent">
                  {grant.role === "Owner" ? (
                    <Tooltip content="A gateway must keep at least one owner.">
                      <Button
                        aria-label="A gateway must keep at least one owner."
                        isAriaDisabled
                        isDanger
                        variant="link"
                      >
                        Remove access
                      </Button>
                    </Tooltip>
                  ) : (
                    <Button isDanger variant="link">
                      Remove access
                    </Button>
                  )}
                </Td>
              </Tr>
            ))}
          </Tbody>
        </Table>
      </StackItem>
      <StackItem>
        <Toolbar>
          <ToolbarContent>
            <ToolbarItem variant="pagination">
              <MockPagination
                itemCount={accessGrants.length}
                variant="bottom"
              />
            </ToolbarItem>
          </ToolbarContent>
        </Toolbar>
      </StackItem>
      <StackItem>
        <Card className={styles.automationCard}>
          <CardHeader>
            <CardTitle>
              <Title headingLevel="h3" size="lg">
                Need automation access?
              </Title>
            </CardTitle>
          </CardHeader>
          <CardBody>
            <Content component="p">
              Use a service account for CI/CD pipelines and bots. Service
              accounts connect with client credentials instead of a personal
              user account.
            </Content>
            <Button onClick={onOpenServiceAccounts} variant="link">
              Manage service accounts
            </Button>
          </CardBody>
        </Card>
      </StackItem>
    </Stack>
  );
}

function ServiceAccountsTab({
  attention,
  isProvisioning = false,
  onOpenSetupInstructions,
}: {
  attention?: GatewayDetailAttention;
  isProvisioning?: boolean;
  onOpenSetupInstructions: (accountName: string) => void;
}) {
  const [expandedAccount, setExpandedAccount] = useState<string>();
  const [openMenu, setOpenMenu] = useState<string>();
  const [sortColumn, setSortColumn] = useState("name");
  const [sortDirection, setSortDirection] = useState<"asc" | "desc">("asc");
  const columns = ["name", "role", "status", "expiresAt"];
  const activeSortIndex = Math.max(0, columns.indexOf(sortColumn));
  const getSortParams = (columnIndex: number) => ({
    columnIndex,
    onSort: (_event: unknown, _index: number, direction: "asc" | "desc") => {
      setSortColumn(columns[columnIndex] ?? "name");
      setSortDirection(direction);
    },
    sortBy: {
      direction: sortDirection,
      index: activeSortIndex,
    },
  });

  return (
    <Stack hasGutter>
      <StackItem>
        <Flex justifyContent={{ default: "justifyContentSpaceBetween" }}>
          <FlexItem>
            <Title headingLevel="h2" size="xl">
              Service accounts
            </Title>
            <Content component="p">
              Use service accounts for automation. Each account exchanges client
              credentials for short-lived JWTs that work only with this gateway.
            </Content>
          </FlexItem>
          <FlexItem>
            <Button
              aria-label="Refresh service accounts"
              icon={<SyncAltIcon />}
              title="Refresh service accounts"
              variant="plain"
            />
          </FlexItem>
        </Flex>
      </StackItem>
      {isProvisioning ? (
        <StackItem>
          <Alert
            isInline
            title="Service accounts can be created after provisioning completes"
            variant="info"
          >
            A service account is tied to this gateway's OpenShell identity. The
            gateway must be running and healthy before its credentials can be
            created and verified.
          </Alert>
        </StackItem>
      ) : null}
      {attention === "service-account-expiring" &&
      expiringServiceAccounts.length > 0 ? (
        <StackItem>
          <Alert
            isInline
            title={`${expiringServiceAccounts.length} service ${
              expiringServiceAccounts.length === 1 ? "account" : "accounts"
            } ${
              expiringServiceAccounts.length === 1 ? "expires" : "expire"
            } within 7 days`}
            variant="warning"
          >
            Review the affected accounts and rotate credentials before
            expiration.
          </Alert>
        </StackItem>
      ) : null}
      <StackItem>
        <Toolbar>
          <ToolbarContent>
            <ToolbarItem>
              <SearchInput placeholder="Filter by name or client ID" />
            </ToolbarItem>
            <ToolbarItem>
              <Button isDisabled={isProvisioning} variant="primary">
                Create service account
              </Button>
            </ToolbarItem>
            <ToolbarItem variant="pagination">
              <MockPagination itemCount={serviceAccounts.length} />
            </ToolbarItem>
          </ToolbarContent>
        </Toolbar>
      </StackItem>
      <StackItem>
        <Table aria-label="Service accounts" isExpandable variant="compact">
          <Thead>
            <Tr>
              <Th screenReaderText="Row expansion" />
              <Th sort={getSortParams(0)} width={30}>
                Service account name
              </Th>
              <Th width={10}>Setup</Th>
              <Th sort={getSortParams(1)} width={15}>
                OpenShell role
              </Th>
              <Th sort={getSortParams(2)} width={15}>
                Status
              </Th>
              <Th sort={getSortParams(3)} width={30}>
                Expiration
              </Th>
              <Th screenReaderText="Actions" />
            </Tr>
          </Thead>
          {serviceAccounts.map((account, rowIndex) => {
            const isExpanded = expandedAccount === account.name;
            return (
              <Tbody key={account.name} isExpanded={isExpanded}>
                <Tr>
                  <Td
                    expand={{
                      expandId: `service-account-${account.name}`,
                      isExpanded,
                      rowIndex,
                      onToggle: () => {
                        setExpandedAccount(
                          isExpanded ? undefined : account.name,
                        );
                      },
                    }}
                  />
                  <Td dataLabel="Service account name">{account.name}</Td>
                  <Td dataLabel="Setup">
                    <Tooltip
                      content={`Open setup instructions for ${account.name}`}
                    >
                      <Button
                        aria-label={`Open setup instructions for ${account.name}`}
                        icon={<CodeIcon />}
                        onClick={() => onOpenSetupInstructions(account.name)}
                        variant="plain"
                      />
                    </Tooltip>
                  </Td>
                  <Td dataLabel="OpenShell role">{account.role}</Td>
                  <Td dataLabel="Status">
                    <HealthStatus
                      appearance={
                        account.status === "Ready" ? "good" : "warning"
                      }
                      label={account.status}
                    />
                  </Td>
                  <Td dataLabel="Expiration">{account.expiration}</Td>
                  <Td isActionCell modifier="fitContent">
                    <Dropdown
                      isOpen={openMenu === account.name}
                      onOpenChange={() => setOpenMenu(undefined)}
                      popperProps={{ position: "right" }}
                      toggle={(toggleRef) => (
                        <MenuToggle
                          aria-label={`Actions for service account ${account.name}`}
                          isExpanded={openMenu === account.name}
                          onClick={() =>
                            setOpenMenu(
                              openMenu === account.name
                                ? undefined
                                : account.name,
                            )
                          }
                          ref={toggleRef}
                          variant="plain"
                        >
                          <EllipsisVIcon />
                        </MenuToggle>
                      )}
                    >
                      <DropdownList>
                        <DropdownItem
                          onClick={() => onOpenSetupInstructions(account.name)}
                        >
                          View setup instructions
                        </DropdownItem>
                        <DropdownItem>Edit role</DropdownItem>
                        <DropdownItem>Revoke service account</DropdownItem>
                        <DropdownItem isDanger>
                          Delete service account
                        </DropdownItem>
                      </DropdownList>
                    </Dropdown>
                  </Td>
                </Tr>
                <Tr isExpanded={isExpanded}>
                  <Td colSpan={7} dataLabel="Account details" noPadding>
                    <ExpandableRowContent>
                      <DescriptionList isCompact className={styles.expandedRow}>
                        <DescriptionListGroup>
                          <DescriptionListTerm>Description</DescriptionListTerm>
                          <DescriptionListDescription>
                            {account.description ?? "None"}
                          </DescriptionListDescription>
                        </DescriptionListGroup>
                        <DescriptionListGroup>
                          <DescriptionListTerm>Client ID</DescriptionListTerm>
                          <DescriptionListDescription>
                            <ClipboardCopy isCode isReadOnly>
                              {account.clientId}
                            </ClipboardCopy>
                          </DescriptionListDescription>
                        </DescriptionListGroup>
                        <DescriptionListGroup>
                          <DescriptionListTerm>
                            Principal subject
                          </DescriptionListTerm>
                          <DescriptionListDescription>
                            <Stack hasGutter>
                              <StackItem>
                                <ClipboardCopy isCode isReadOnly>
                                  {account.subject}
                                </ClipboardCopy>
                              </StackItem>
                              <StackItem>
                                <Content component="p">
                                  This stable ID is the JWT subject (sub). Use
                                  it to grant OpenShell workspace access.
                                </Content>
                              </StackItem>
                            </Stack>
                          </DescriptionListDescription>
                        </DescriptionListGroup>
                        <DescriptionListGroup>
                          <DescriptionListTerm>Workspaces</DescriptionListTerm>
                          <DescriptionListDescription>
                            {account.workspaces.join(", ")}
                          </DescriptionListDescription>
                        </DescriptionListGroup>
                        <DescriptionListGroup>
                          <DescriptionListTerm>Created</DescriptionListTerm>
                          <DescriptionListDescription>
                            <time dateTime="2026-10-28T22:35:00">
                              {account.createdAt}
                            </time>
                          </DescriptionListDescription>
                        </DescriptionListGroup>
                      </DescriptionList>
                    </ExpandableRowContent>
                  </Td>
                </Tr>
              </Tbody>
            );
          })}
        </Table>
      </StackItem>
      <StackItem>
        <Toolbar>
          <ToolbarContent>
            <ToolbarItem variant="pagination">
              <MockPagination
                itemCount={serviceAccounts.length}
                variant="bottom"
              />
            </ToolbarItem>
          </ToolbarContent>
        </Toolbar>
      </StackItem>
    </Stack>
  );
}

function DetailsTab({
  attention,
  onChangeTab,
}: {
  attention?: GatewayDetailAttention;
  onChangeTab: (tab: GatewayDetailTab) => void;
}) {
  const placement = {
    region: "us-east-1" as string | undefined,
    runtime: "Kata" as string | undefined,
  };

  return (
    <Stack hasGutter>
      <StackItem>
        <Flex justifyContent={{ default: "justifyContentSpaceBetween" }}>
          <FlexItem>
            <Title headingLevel="h2" size="xl">
              Details
            </Title>
            <Content component="p">
              Review the gateway's current state and configuration.
            </Content>
          </FlexItem>
          <FlexItem>
            <Button
              aria-label="Refresh gateway details"
              icon={<SyncAltIcon />}
              title="Refresh gateway details"
              variant="plain"
            />
          </FlexItem>
        </Flex>
      </StackItem>
      {attention === "gateway-health" ? (
        <StackItem>
          <Alert
            actionLinks={
              <Button
                component="a"
                href={`${mockGateway.consoleUrl}/sandboxes`}
                isInline
                target="_blank"
                variant="link"
              >
                View all sandboxes
              </Button>
            }
            isInline
            title="Gateway needs attention"
            variant="warning"
          >
            Connection health is degraded. 3 active sandboxes may be affected.
          </Alert>
        </StackItem>
      ) : null}
      {attention === "sandbox-expiring" ? (
        <StackItem>
          <Alert
            actionLinks={
              <Button
                component="a"
                href="https://payments-gateway.example.com/console"
                isInline
                target="_blank"
                variant="link"
              >
                Open gateway console
              </Button>
            }
            isInline
            title="2 sandboxes expire soon"
            variant="warning"
          >
            View all sandboxes to investigate or extend their lifecycle.
          </Alert>
        </StackItem>
      ) : null}
      {attention === "sandbox-idle" ? (
        <StackItem>
          <Alert
            actionLinks={
              <Button
                component="a"
                href={`${mockGateway.consoleUrl}/sandboxes`}
                isInline
                target="_blank"
                variant="link"
              >
                View all sandboxes
              </Button>
            }
            isInline
            title="5 sandboxes are idle"
            variant="warning"
          >
            View all sandboxes to review activity and stop unused sandboxes.
          </Alert>
        </StackItem>
      ) : null}
      {attention === "service-account-expiring" &&
      expiringServiceAccounts.length > 0 ? (
        <StackItem>
          <Alert
            actionLinks={
              <Button
                isInline
                onClick={() => onChangeTab("service-accounts")}
                variant="link"
              >
                Review service accounts
              </Button>
            }
            isInline
            title={`${expiringServiceAccounts.length} service ${
              expiringServiceAccounts.length === 1 ? "account" : "accounts"
            } ${
              expiringServiceAccounts.length === 1 ? "expires" : "expire"
            } within 7 days`}
            variant="warning"
          >
            Review and rotate credentials before access stops.
          </Alert>
        </StackItem>
      ) : null}
      <StackItem>
        <Card>
          <CardHeader>
            <CardTitle>
              <Title headingLevel="h2" size="xl">
                Overview
              </Title>
            </CardTitle>
          </CardHeader>
          <CardBody>
            <Grid hasGutter>
              <GridItem lg={6} span={12}>
                <DescriptionList>
                  <DescriptionListGroup>
                    <DescriptionListTerm>Status</DescriptionListTerm>
                    <DescriptionListDescription>
                      <HealthStatus label="Healthy" />
                    </DescriptionListDescription>
                  </DescriptionListGroup>
                  <DescriptionListGroup>
                    <DescriptionListTerm>Active sandboxes</DescriptionListTerm>
                    <DescriptionListDescription>3</DescriptionListDescription>
                  </DescriptionListGroup>
                  <DescriptionListGroup>
                    <DescriptionListTerm>Cluster type</DescriptionListTerm>
                    <DescriptionListDescription>
                      IBM Public Kata
                    </DescriptionListDescription>
                  </DescriptionListGroup>
                </DescriptionList>
              </GridItem>
              <GridItem lg={6} span={12}>
                <DescriptionList>
                  <DescriptionListGroup>
                    <DescriptionListTerm>Version</DescriptionListTerm>
                    <DescriptionListDescription>
                      1.8.0
                    </DescriptionListDescription>
                  </DescriptionListGroup>
                  <DescriptionListGroup>
                    <DescriptionListTerm>Created</DescriptionListTerm>
                    <DescriptionListDescription>
                      Oct 1, 2026, 9:30 AM UTC
                    </DescriptionListDescription>
                  </DescriptionListGroup>
                  <DescriptionListGroup>
                    <DescriptionListTerm>Created by</DescriptionListTerm>
                    <DescriptionListDescription>
                      kim-developer
                    </DescriptionListDescription>
                  </DescriptionListGroup>
                </DescriptionList>
              </GridItem>
              <GridItem span={12}>
                <Flex
                  alignItems={{ default: "alignItemsFlexEnd" }}
                  flexWrap={{ default: "wrap" }}
                  justifyContent={{
                    default: "justifyContentSpaceBetween",
                  }}
                >
                  <FlexItem grow={{ default: "grow" }}>
                    <DescriptionList>
                      <DescriptionListGroup>
                        <DescriptionListTerm>
                          Gateway endpoint
                        </DescriptionListTerm>
                        <DescriptionListDescription>
                          <ClipboardCopy
                            className={styles.endpointCopy}
                            isCode
                            isReadOnly
                          >
                            https://payments-gateway.example.com:443
                          </ClipboardCopy>
                        </DescriptionListDescription>
                      </DescriptionListGroup>
                    </DescriptionList>
                  </FlexItem>
                  <FlexItem>
                    <Button
                      component="a"
                      href="https://payments-gateway.example.com/console"
                      icon={<ExternalLinkAltIcon />}
                      iconPosition="end"
                      target="_blank"
                      variant="secondary"
                    >
                      Open gateway console
                    </Button>
                  </FlexItem>
                </Flex>
              </GridItem>
            </Grid>
          </CardBody>
        </Card>
      </StackItem>
      <StackItem>
        <Grid hasGutter>
          <GridItem lg={6} span={12}>
            <Card>
              <CardHeader>
                <CardTitle>
                  <Title headingLevel="h2" size="xl">
                    Identity
                  </Title>
                </CardTitle>
              </CardHeader>
              <CardBody>
                <DescriptionList>
                  <DescriptionListGroup>
                    <DescriptionListTerm>Gateway ID</DescriptionListTerm>
                    <DescriptionListDescription>
                      gateway-payments
                    </DescriptionListDescription>
                  </DescriptionListGroup>
                  <DescriptionListGroup>
                    <DescriptionListTerm>Namespace</DescriptionListTerm>
                    <DescriptionListDescription>
                      openshell
                    </DescriptionListDescription>
                  </DescriptionListGroup>
                  <DescriptionListGroup>
                    <DescriptionListTerm>OIDC issuer</DescriptionListTerm>
                    <DescriptionListDescription>
                      https://sso.example.com/realms/openshell
                    </DescriptionListDescription>
                  </DescriptionListGroup>
                  <DescriptionListGroup>
                    <DescriptionListTerm>OIDC client ID</DescriptionListTerm>
                    <DescriptionListDescription>
                      payments-client
                    </DescriptionListDescription>
                  </DescriptionListGroup>
                  <DescriptionListGroup>
                    <DescriptionListTerm>OIDC audience</DescriptionListTerm>
                    <DescriptionListDescription>
                      openshell-payments
                    </DescriptionListDescription>
                  </DescriptionListGroup>
                </DescriptionList>
              </CardBody>
            </Card>
          </GridItem>
          <GridItem lg={6} span={12}>
            <Card>
              <CardHeader>
                <CardTitle>
                  <Title headingLevel="h2" size="xl">
                    Placement
                  </Title>
                </CardTitle>
              </CardHeader>
              <CardBody>
                <DescriptionList>
                  <DescriptionListGroup>
                    <DescriptionListTerm>Cluster type</DescriptionListTerm>
                    <DescriptionListDescription>
                      IBM Public Kata
                    </DescriptionListDescription>
                  </DescriptionListGroup>
                  <DescriptionListGroup>
                    <DescriptionListTerm>Cluster name</DescriptionListTerm>
                    <DescriptionListDescription>
                      aws-us-east-1-production
                    </DescriptionListDescription>
                  </DescriptionListGroup>
                  {placement.region ? (
                    <DescriptionListGroup>
                      <DescriptionListTerm>Region</DescriptionListTerm>
                      <DescriptionListDescription>
                        {placement.region}
                      </DescriptionListDescription>
                    </DescriptionListGroup>
                  ) : null}
                  {placement.runtime ? (
                    <DescriptionListGroup>
                      <DescriptionListTerm>Runtime</DescriptionListTerm>
                      <DescriptionListDescription>
                        {placement.runtime}
                      </DescriptionListDescription>
                    </DescriptionListGroup>
                  ) : null}
                </DescriptionList>
              </CardBody>
            </Card>
          </GridItem>
        </Grid>
      </StackItem>
    </Stack>
  );
}

function DetailDialog({
  modal,
  workspaceUsername = accessGrants[0]?.username,
  serviceAccountName = serviceAccounts.find(
    ({ role }) => role === "openshell-user",
  )?.name ?? serviceAccounts[0]?.name,
  serviceAccountSecretAvailable = true,
  onClose,
}: {
  modal: GatewayDetailModal;
  workspaceUsername?: string;
  serviceAccountName?: string;
  serviceAccountSecretAvailable?: boolean;
  onClose: () => void;
}) {
  const small = ModalVariant.small;
  const large = ModalVariant.large;
  const [editServiceAccountRole, setEditServiceAccountRole] =
    useState("openshell-user");
  const [isEditServiceAccountRoleOpen, setIsEditServiceAccountRoleOpen] =
    useState(false);
  const [editServiceAccountDescription, setEditServiceAccountDescription] =
    useState("");
  const [isServiceAccountJwtExpanded, setIsServiceAccountJwtExpanded] =
    useState(false);

  if (modal === "rename-gateway") {
    return (
      <Modal isOpen onClose={onClose} variant={small}>
        <ModalHeader title="Rename gateway" />
        <ModalBody>
          <Form>
            <FormGroup fieldId="gateway-name" isRequired label="Gateway name">
              <TextInput
                id="gateway-name"
                isRequired
                value="payments-gateway"
              />
            </FormGroup>
          </Form>
        </ModalBody>
        <ModalFooter>
          <Button variant="primary">Rename gateway</Button>
          <Button onClick={onClose} variant="link">
            Cancel
          </Button>
        </ModalFooter>
      </Modal>
    );
  }

  if (modal === "delete-gateway") {
    return (
      <Modal isOpen onClose={onClose} variant={small}>
        <ModalHeader
          title="Delete payments-gateway?"
          titleIconVariant="warning"
        />
        <ModalBody>
          <Stack hasGutter>
            <StackItem>
              Deleting payments-gateway will permanently remove the gateway.
              This action cannot be undone.
            </StackItem>
            <StackItem>
              <Alert
                isInline
                title="3 active sandboxes use this gateway"
                variant="warning"
              />
            </StackItem>
          </Stack>
        </ModalBody>
        <ModalFooter>
          <Button variant="danger">Delete gateway</Button>
          <Button onClick={onClose} variant="link">
            Cancel
          </Button>
        </ModalFooter>
      </Modal>
    );
  }

  if (modal === "add-users") {
    return (
      <Modal isOpen onClose={onClose} variant={small}>
        <ModalHeader title="Add user" />
        <ModalBody>
          <Form>
            <Stack hasGutter>
              <StackItem>
                <Content component="p">
                  Search the identity directory for a user to add to the
                  gateway.
                </Content>
              </StackItem>
              <StackItem>
                <MenuToggle isFullWidth variant="typeahead">
                  <TextInputGroup isPlain>
                    <TextInputGroupMain
                      autoComplete="off"
                      aria-autocomplete="list"
                      aria-label="Find people"
                      icon={<SearchIcon />}
                      placeholder="Find people"
                      role="combobox"
                      value=""
                    />
                  </TextInputGroup>
                </MenuToggle>
              </StackItem>
            </Stack>
          </Form>
        </ModalBody>
        <ModalFooter>
          <Button isDisabled variant="primary">
            Grant access
          </Button>
          <Button onClick={onClose} variant="link">
            Cancel
          </Button>
        </ModalFooter>
      </Modal>
    );
  }

  if (modal === "workspace-access") {
    const grant =
      accessGrants.find(({ username }) => username === workspaceUsername) ??
      accessGrants.find(({ role }) => role === "User");
    const workspaceCommand = grant
      ? workspaceMembershipCommand(grant.subject, "default")
      : "";
    return (
      <Modal isOpen onClose={onClose} variant={ModalVariant.medium}>
        <ModalHeader title="Grant workspace access" />
        <ModalBody>
          <Stack hasGutter>
            <StackItem>
              <Content component="p">
                {grant?.name} has been added to payments-gateway, but they
                cannot use it yet. Add them to a workspace inside the gateway
                before they can sign in and run sandboxes.
              </Content>
            </StackItem>
            <StackItem>
              <Content component="p">
                You can add them in the gateway console or run this command in a
                terminal. The command uses the default workspace; change{" "}
                <code>WORKSPACE_NAME</code> if they should use a different
                workspace.
              </Content>
            </StackItem>
            <StackItem>
              <div className={styles.steps}>
                <CommandBlock
                  copyAriaLabel="Copy workspace access command"
                  copyText={workspaceCommand}
                />
              </div>
            </StackItem>
          </Stack>
        </ModalBody>
        <ModalFooter>
          <Button onClick={onClose} variant="primary">
            Close
          </Button>
        </ModalFooter>
      </Modal>
    );
  }

  if (modal === "change-own-role") {
    return (
      <Modal isOpen onClose={onClose} variant={small}>
        <ModalHeader title="Change your own role?" />
        <ModalBody>
          You are about to change your own role on this gateway to Admin. This
          may reduce your own access. Do you want to continue?
        </ModalBody>
        <ModalFooter>
          <Button variant="primary">Change my role</Button>
          <Button onClick={onClose} variant="link">
            Cancel
          </Button>
        </ModalFooter>
      </Modal>
    );
  }

  if (modal === "remove-access") {
    return (
      <Modal isOpen onClose={onClose} variant={small}>
        <ModalHeader title="Remove access for Jordan Automation?" />
        <ModalBody>This revokes the user's access to the gateway.</ModalBody>
        <ModalFooter>
          <Button variant="primary">Remove access</Button>
          <Button onClick={onClose} variant="link">
            Cancel
          </Button>
        </ModalFooter>
      </Modal>
    );
  }

  if (modal === "create-service-account") {
    return (
      <Modal isOpen onClose={onClose} variant={large}>
        <ModalHeader title="Create service account" />
        <ModalBody>
          <Form>
            <Stack hasGutter>
              <StackItem>
                <FormGroup
                  fieldId="service-account-name"
                  isRequired
                  label="Service account name"
                >
                  <TextInput id="service-account-name" isRequired />
                </FormGroup>
              </StackItem>
              <StackItem>
                <FormGroup
                  fieldId="service-account-description"
                  label="Description (optional)"
                >
                  <TextArea id="service-account-description" />
                </FormGroup>
              </StackItem>
              <StackItem>
                <FormGroup
                  fieldId="service-account-role"
                  label="OpenShell role"
                >
                  <Select
                    id="service-account-role"
                    isOpen={false}
                    onOpenChange={() => undefined}
                    selected="openshell-user"
                    toggle={(toggleRef) => (
                      <MenuToggle isFullWidth ref={toggleRef}>
                        openshell-user
                      </MenuToggle>
                    )}
                  >
                    <SelectList>
                      <SelectOption isSelected={false} value="openshell-admin">
                        openshell-admin
                      </SelectOption>
                      <SelectOption isSelected value="openshell-user">
                        openshell-user
                      </SelectOption>
                    </SelectList>
                  </Select>
                </FormGroup>
              </StackItem>
              <StackItem>
                <FormGroup
                  fieldId="service-account-expiration"
                  isRequired
                  label="Expiration"
                >
                  <Select
                    isOpen={false}
                    onOpenChange={() => undefined}
                    selected="90 days"
                    toggle={(toggleRef) => (
                      <MenuToggle isFullWidth ref={toggleRef}>
                        90 days
                      </MenuToggle>
                    )}
                  >
                    <SelectList>
                      <SelectOption value="30">30 days</SelectOption>
                      <SelectOption value="60">60 days</SelectOption>
                      <SelectOption isSelected value="90">
                        90 days
                      </SelectOption>
                    </SelectList>
                  </Select>
                  <HelperText>
                    <HelperTextItem>
                      Expires Jan 27, 2027, 2:56:03 PM CST.
                    </HelperTextItem>
                    <HelperTextItem>
                      Account expiration stops future token grants. Each access
                      token has a separate short lifetime.
                    </HelperTextItem>
                  </HelperText>
                </FormGroup>
              </StackItem>
            </Stack>
          </Form>
        </ModalBody>
        <ModalFooter>
          <Button variant="primary">Create service account</Button>
          <Button onClick={onClose} variant="link">
            Cancel
          </Button>
        </ModalFooter>
      </Modal>
    );
  }

  if (modal === "service-account-setup") {
    const account =
      serviceAccounts.find(({ name }) => name === serviceAccountName) ??
      serviceAccounts[0];
    return (
      <Modal isOpen onClose={onClose} variant={large}>
        <ModalHeader title={`Set up ${account.name}`} />
        <ModalBody>
          <Stack hasGutter>
            {serviceAccountSecretAvailable ? (
              <StackItem>
                <Alert
                  isInline
                  title="Save the client secret now"
                  variant="warning"
                >
                  HyperShell cannot show the client secret again. Store it in
                  your organization's secret manager before you close this
                  modal.
                </Alert>
              </StackItem>
            ) : null}
            <StackItem>
              <DescriptionList isCompact>
                <DescriptionListGroup>
                  <DescriptionListTerm>Client ID</DescriptionListTerm>
                  <DescriptionListDescription>
                    <ClipboardCopy isCode isReadOnly>
                      {account.clientId}
                    </ClipboardCopy>
                  </DescriptionListDescription>
                </DescriptionListGroup>
                {serviceAccountSecretAvailable ? (
                  <DescriptionListGroup>
                    <DescriptionListTerm>Client secret</DescriptionListTerm>
                    <DescriptionListDescription>
                      <ClipboardCopy isCode isReadOnly>
                        {mockServiceAccountSecret}
                      </ClipboardCopy>
                    </DescriptionListDescription>
                  </DescriptionListGroup>
                ) : null}
                <DescriptionListGroup>
                  <DescriptionListTerm>Principal subject</DescriptionListTerm>
                  <DescriptionListDescription>
                    <ClipboardCopy isCode isReadOnly>
                      {account.subject}
                    </ClipboardCopy>
                  </DescriptionListDescription>
                </DescriptionListGroup>
              </DescriptionList>
            </StackItem>
            {account.role === "openshell-user" ? (
              <StackItem>
                <Title headingLevel="h3" size="md">
                  Grant workspace access
                </Title>
                <Content component="p">
                  {account.name} has been added to payments-gateway, but it
                  cannot use the gateway yet. Add its principal subject to a
                  workspace inside the gateway before it can run sandboxes.
                </Content>
                <Content component="p">
                  You can add it in the gateway console or run this command in a
                  terminal. The command uses the default workspace; change{" "}
                  <code>WORKSPACE_NAME</code> if it should use a different
                  workspace.
                </Content>
                <div className={styles.steps}>
                  <CommandBlock
                    copyAriaLabel={`Copy workspace access command for ${account.name}`}
                    copyText={workspaceMembershipCommand(
                      account.subject,
                      "default",
                    )}
                  />
                </div>
              </StackItem>
            ) : null}
            <StackItem>
              <ExpandableSection
                isExpanded={isServiceAccountJwtExpanded}
                onToggle={(_event, expanded) => {
                  setIsServiceAccountJwtExpanded(expanded);
                }}
                toggleText="Request an access token"
                toggleWrapper="h3"
              >
                <Stack hasGutter>
                  <StackItem>
                    <Content component="p">
                      Automation sends the client ID and secret to Keycloak,
                      which returns a signed JWT that expires in about five
                      minutes. The JWT is useful because the secret stays in
                      your secret manager while the automation process presents
                      only a short-lived token. This exchange does not
                      deactivate the client secret or other credentials.
                    </Content>
                  </StackItem>
                  <StackItem>
                    <div className={styles.steps}>
                      <CommandBlock
                        copyAriaLabel={`Copy access token command for ${account.name}`}
                        copyText={serviceAccountJwtCommand(account.clientId)}
                      />
                    </div>
                  </StackItem>
                </Stack>
              </ExpandableSection>
            </StackItem>
          </Stack>
        </ModalBody>
        <ModalFooter>
          <Button onClick={onClose} variant="primary">
            Close
          </Button>
        </ModalFooter>
      </Modal>
    );
  }

  if (modal === "edit-service-account") {
    return (
      <Modal isOpen onClose={onClose} variant={small}>
        <ModalHeader title="Edit ci-pipeline" />
        <ModalBody>
          <p>Change the OpenShell role or description of ci-pipeline.</p>
          <Form>
            <FormGroup
              fieldId="service-account-edit-role"
              isRequired
              label="OpenShell role"
            >
              <Select
                id="service-account-edit-role"
                isOpen={isEditServiceAccountRoleOpen}
                onOpenChange={setIsEditServiceAccountRoleOpen}
                onSelect={(_event, value) => {
                  if (typeof value === "string") {
                    setEditServiceAccountRole(value);
                  }
                  setIsEditServiceAccountRoleOpen(false);
                }}
                selected={editServiceAccountRole}
                toggle={(toggleRef) => (
                  <MenuToggle
                    aria-label="OpenShell role"
                    isExpanded={isEditServiceAccountRoleOpen}
                    isFullWidth
                    onClick={() => {
                      setIsEditServiceAccountRoleOpen((open) => !open);
                    }}
                    ref={toggleRef}
                  >
                    {editServiceAccountRole}
                  </MenuToggle>
                )}
              >
                <SelectList>
                  <SelectOption
                    description="Can perform OpenShell administrative operations on this gateway."
                    value="openshell-admin"
                  >
                    openshell-admin
                  </SelectOption>
                  <SelectOption
                    description="Can authenticate as openshell-user. A gateway administrator must separately add its subject to each workspace it needs."
                    isSelected={editServiceAccountRole === "openshell-user"}
                    value="openshell-user"
                  >
                    openshell-user
                  </SelectOption>
                </SelectList>
              </Select>
            </FormGroup>
            <FormGroup
              fieldId="service-account-edit-description"
              label="Description (optional)"
            >
              <TextArea
                id="service-account-edit-description"
                onChange={(_event, value) => {
                  setEditServiceAccountDescription(value);
                }}
                rows={3}
                value={editServiceAccountDescription}
              />
            </FormGroup>
          </Form>
          <Alert
            className={styles.editAccountAlert}
            isInline
            title="Role changes apply to new tokens"
            variant="info"
          >
            Existing tokens expire within a few minutes.
          </Alert>
        </ModalBody>
        <ModalFooter>
          <Button variant="primary">Save changes</Button>
          <Button onClick={onClose} variant="link">
            Cancel
          </Button>
        </ModalFooter>
      </Modal>
    );
  }

  if (modal === "revoke-service-account") {
    return (
      <Modal isOpen onClose={onClose} variant={small}>
        <ModalHeader title="Revoke ci-pipeline?" titleIconVariant="warning" />
        <ModalBody>
          <Stack hasGutter>
            <StackItem>
              New access tokens will permanently stop being issued. Tokens
              already issued will work until they expire, usually within five
              minutes.
            </StackItem>
            <StackItem>
              The service account will remain listed for audit purposes and can
              be deleted later.
            </StackItem>
          </Stack>
        </ModalBody>
        <ModalFooter>
          <Button variant="danger">Revoke service account</Button>
          <Button onClick={onClose} variant="link">
            Cancel
          </Button>
        </ModalFooter>
      </Modal>
    );
  }

  return (
    <Modal isOpen onClose={onClose} variant={small}>
      <ModalHeader title="Delete ci-pipeline?" titleIconVariant="warning" />
      <ModalBody>
        <Stack hasGutter>
          <StackItem>
            This permanently deletes ci-pipeline and its sign-in identity. A
            ready account is revoked first.
          </StackItem>
          <StackItem>
            Tokens already issued will work until they expire, usually within
            five minutes.
          </StackItem>
        </Stack>
      </ModalBody>
      <ModalFooter>
        <Button variant="danger">Delete service account</Button>
        <Button onClick={onClose} variant="link">
          Cancel
        </Button>
      </ModalFooter>
    </Modal>
  );
}

export function GatewayDetailContent({
  activeTab: initialTab = "details",
  attention,
  isProvisioning = false,
  modal: initialModal,
}: {
  activeTab?: GatewayDetailTab;
  attention?: GatewayDetailAttention;
  isProvisioning?: boolean;
  modal?: GatewayDetailModal;
}) {
  const [activeTab, setActiveTab] = useState(initialTab);
  const [modal, setModal] = useState(initialModal);
  const [isActionsOpen, setIsActionsOpen] = useState(false);
  const [setupAccountName, setSetupAccountName] = useState(
    serviceAccounts.find(({ role }) => role === "openshell-user")?.name ??
      serviceAccounts[0]?.name,
  );
  const [workspaceUsername, setWorkspaceUsername] = useState(
    accessGrants.find(({ role }) => role === "User")?.username,
  );
  const [setupAccountSecretAvailable, setSetupAccountSecretAvailable] =
    useState(true);

  return (
    <>
      <PageSection
        data-page-region="header"
        hasBodyWrapper={false}
        variant="default"
      >
        <Flex
          alignItems={{ default: "alignItemsFlexStart" }}
          flexWrap={{ default: "wrap" }}
          justifyContent={{ default: "justifyContentSpaceBetween" }}
        >
          <FlexItem>
            <Flex alignItems={{ default: "alignItemsCenter" }}>
              <FlexItem>
                <Title headingLevel="h1" size="2xl">
                  payments-gateway
                </Title>
              </FlexItem>
              <FlexItem>
                <HealthStatus
                  appearance={isProvisioning ? "warning" : "good"}
                  label={isProvisioning ? "Provisioning" : "Healthy"}
                />
              </FlexItem>
              <FlexItem>
                <Label color="grey">1.8.0</Label>
              </FlexItem>
            </Flex>
          </FlexItem>
          <FlexItem>
            <ActionList>
              {!isProvisioning ? (
                <ActionListItem>
                  <Button
                    component="a"
                    href="https://payments-gateway.example.com/console"
                    icon={<ExternalLinkAltIcon />}
                    iconPosition="end"
                    target="_blank"
                    variant="primary"
                  >
                    Open gateway console
                  </Button>
                </ActionListItem>
              ) : null}
              <ActionListItem>
                <Dropdown
                  isOpen={isActionsOpen}
                  onOpenChange={setIsActionsOpen}
                  onSelect={() => setIsActionsOpen(false)}
                  toggle={(toggleRef) => (
                    <MenuToggle
                      isExpanded={isActionsOpen}
                      onClick={() => setIsActionsOpen((isOpen) => !isOpen)}
                      ref={toggleRef}
                      variant="secondary"
                    >
                      Actions
                    </MenuToggle>
                  )}
                >
                  <DropdownList>
                    <DropdownItem onClick={() => setModal("rename-gateway")}>
                      Rename gateway
                    </DropdownItem>
                    <DropdownItem
                      isDanger
                      onClick={() => setModal("delete-gateway")}
                    >
                      Delete gateway
                    </DropdownItem>
                  </DropdownList>
                </Dropdown>
              </ActionListItem>
            </ActionList>
          </FlexItem>
        </Flex>
      </PageSection>
      {isProvisioning ? (
        <PageSection hasBodyWrapper={false} variant="default">
          <div className={styles.provisioningStepper}>
            <GatewayProvisioningStepper
              conditions={provisioningConditions}
              consoleReady={false}
              phase="Provisioning"
            />
          </div>
        </PageSection>
      ) : null}
      <PageSection
        data-page-region="body"
        hasBodyWrapper={false}
        isFilled
        variant="default"
      >
        <Tabs
          activeKey={activeTab}
          aria-label="Gateway details, connection, user access, and service accounts"
          onSelect={(_event, eventKey) =>
            setActiveTab(eventKey as GatewayDetailTab)
          }
        >
          <Tab eventKey="details" title={<TabTitleText>Details</TabTitleText>}>
            <DetailsTab attention={attention} onChangeTab={setActiveTab} />
          </Tab>
          <Tab
            eventKey="connection"
            title={<TabTitleText>Connection</TabTitleText>}
          >
            <ProductionConnectionTab onChangeTab={setActiveTab} />
          </Tab>
          <Tab
            eventKey="manage-access"
            title={<TabTitleText>User access</TabTitleText>}
          >
            <AccessTab
              isProvisioning={isProvisioning}
              onOpenServiceAccounts={() => {
                setActiveTab("service-accounts");
              }}
              onOpenWorkspaceAccess={(username) => {
                setWorkspaceUsername(username);
                setModal("workspace-access");
              }}
            />
          </Tab>
          <Tab
            eventKey="service-accounts"
            title={<TabTitleText>Service accounts</TabTitleText>}
          >
            <ServiceAccountsTab
              attention={attention}
              isProvisioning={isProvisioning}
              onOpenSetupInstructions={(accountName) => {
                setSetupAccountName(accountName);
                setSetupAccountSecretAvailable(false);
                setModal("service-account-setup");
              }}
            />
          </Tab>
        </Tabs>
      </PageSection>
      {modal ? (
        <DetailDialog
          modal={modal}
          onClose={() => setModal(undefined)}
          workspaceUsername={workspaceUsername}
          serviceAccountName={setupAccountName}
          serviceAccountSecretAvailable={setupAccountSecretAvailable}
        />
      ) : null}
    </>
  );
}

export function GatewayDetailMockup({
  activeTab,
  attention,
  isProvisioning,
  modal,
}: {
  activeTab?: GatewayDetailTab;
  attention?: GatewayDetailAttention;
  isProvisioning?: boolean;
  modal?: GatewayDetailModal;
}) {
  return (
    <MockupShell activeSection="gateways">
      <GatewayDetailPageSurface
        activeTab={activeTab}
        attention={attention}
        isProvisioning={isProvisioning}
        modal={modal}
      />
    </MockupShell>
  );
}
