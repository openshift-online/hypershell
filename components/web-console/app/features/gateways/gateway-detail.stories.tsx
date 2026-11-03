import type {
  GatewayAccessGrantRecord,
  GatewayAccessListRequest,
  GatewayDirectoryUser,
  GatewayOperations,
  GatewayPlacement,
  GatewayPlacementAvailability,
  GatewayRecord,
  GatewayUiNavigation,
  OpenShellGatewayServiceAccountCreateInput,
  OpenShellGatewayServiceAccountListRequest,
  OpenShellGatewayServiceAccountRecord,
} from "@openshift-online/hypershell-gateway-management-ui";
import {
  GatewayPage,
  GatewayUiProvider,
  type GatewayDetailTab,
} from "@openshift-online/hypershell-gateway-management-ui";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";

const gateway: GatewayRecord = {
  activeSandboxCount: 3,
  canDelete: true,
  canEdit: true,
  clusterId: "cluster-aws-us-east-1",
  consoleUrl: "https://payments-gateway.example.com/console",
  createdAt: "2026-10-01T09:30:00Z",
  createdBy: "kim-developer",
  externalDns: "payments-gateway.example.com",
  gatewayVersion: "1.8.0",
  id: "gateway-payments",
  name: "payments-gateway",
  namespace: "openshell",
  oidcAudience: "openshell-payments",
  oidcClientId: "payments-client",
  oidcIssuer: "https://sso.example.com/realms/openshell",
  phase: "Running",
  status: "Ready",
};

const provisioningGateway: GatewayRecord = {
  ...gateway,
  activeSandboxCount: undefined,
  consoleUrl: undefined,
  createdAt: "2026-10-29T14:00:00Z",
  phase: "Provisioning",
  status: "Provisioning",
  provisioningConditions: [
    {
      conditionStatus: "Complete",
      message: "Gateway database and schema are ready.",
      type: "DatabaseReady",
    },
    {
      conditionStatus: "InProgress",
      message: "Waiting for the gateway console endpoint.",
      type: "ConsoleReady",
    },
  ],
};

const placement: GatewayPlacement = {
  id: gateway.clusterId,
  name: "aws-us-east-1-production",
  provider: "AWS",
  region: "us-east-1",
  status: "Ready",
};

const oidcTokenEndpoint = `${gateway.oidcIssuer ?? ""}/protocol/openid-connect/token`;

const accessGrants: GatewayAccessGrantRecord[] = [
  {
    email: "kim@example.com",
    grantedAt: "2026-10-02T10:15:00Z",
    name: "Kim Developer",
    role: "owner",
    roleBindingId: "grant-kim",
    userId: "user-kim",
    username: "kim-developer",
  },
  {
    grantedAt: "2026-10-04T12:00:00Z",
    name: "Alex Operator",
    role: "admin",
    roleBindingId: "grant-alex",
    userId: "user-alex",
    username: "alex-operator",
  },
  {
    email: "jordan@example.com",
    grantedAt: "2026-10-10T16:45:00Z",
    name: "Jordan Automation",
    role: "user",
    roleBindingId: "grant-jordan",
    userId: "user-jordan",
    username: "jordan-automation",
  },
];

const directoryUsers: GatewayDirectoryUser[] = [
  {
    email: "casey@example.com",
    name: "Casey Release",
    subject: "user-casey",
    username: "casey-release",
  },
  {
    email: "riley@example.com",
    name: "Riley Support",
    subject: "user-riley",
    username: "riley-support",
  },
];

const serviceAccounts: OpenShellGatewayServiceAccountRecord[] = [
  {
    clientId: "ci-pipeline-client",
    createdAt: "2026-10-05T08:00:00Z",
    createdByUserId: "user-kim",
    description: "Continuous integration deployment client",
    expiresAt: "2027-10-05T08:00:00Z",
    gatewayId: gateway.id,
    id: "service-account-ci",
    name: "ci-pipeline",
    role: "openshell-admin",
    status: "ready",
    subject: "openshell:service-account:service-account-ci",
    updatedAt: "2026-10-05T08:00:00Z",
  },
  {
    clientId: "observability-client",
    createdAt: "2026-10-12T13:20:00Z",
    createdByUserId: "user-alex",
    description: "Metrics and trace collection client",
    expiresAt: "2026-11-12T13:20:00Z",
    gatewayId: gateway.id,
    id: "service-account-observability",
    name: "observability",
    role: "openshell-user",
    status: "provisioning",
    subject: "openshell:service-account:service-account-observability",
    updatedAt: "2026-10-12T13:20:00Z",
  },
];

const navigation: GatewayUiNavigation = {
  collectionHref: "/gateways",
  createHref: "/gateways/new",
  detailHref: (gatewayId) => `/gateways/${gatewayId}`,
  navigate: () => undefined,
};

const gatewayOperations = {
  changeGatewayAccessRole: (_gatewayId: string, userId: string, role: string) =>
    Promise.resolve(
      accessGrants.find((grant) => grant.userId === userId) ?? {
        grantedAt: new Date().toISOString(),
        role,
        roleBindingId: `grant-${userId}`,
        userId,
        username: userId,
      },
    ),
  createOpenShellGatewayServiceAccount: (
    _gatewayId: string,
    input: OpenShellGatewayServiceAccountCreateInput,
  ) =>
    Promise.resolve({
      credential: {
        accessTokenLifetimeSeconds: 900,
        audience: gateway.oidcAudience,
        clientId: `${input.name}-client`,
        clientSecret: "mock-one-time-client-secret",
        gatewayEndpoint: gateway.externalDns,
        gatewayName: gateway.name,
        issuer: gateway.oidcIssuer,
        tokenEndpoint: oidcTokenEndpoint,
      },
      serviceAccount: {
        ...serviceAccounts[0],
        createdAt: new Date().toISOString(),
        description: input.description,
        expiresAt: input.expiresAt,
        id: `service-account-${input.name}`,
        name: input.name,
        role: input.role,
        status: "ready",
        subject: `openshell:service-account:service-account-${input.name}`,
        updatedAt: new Date().toISOString(),
      },
    }),
  grantGatewayAccess: (
    _gatewayId: string,
    input: { role: string; username: string; subject?: string },
  ) =>
    Promise.resolve({
      grantedAt: new Date().toISOString(),
      role: input.role,
      roleBindingId: `grant-${input.username}`,
      userId: input.subject ?? input.username,
      username: input.username,
    }),
  listGatewayAccess: (_gatewayId: string, request: GatewayAccessListRequest) =>
    Promise.resolve({
      capabilities: {
        callerRole: "owner",
        canManageAccess: true,
        canManageOwners: true,
      },
      items: accessGrants.filter((grant) =>
        request.search
          ? [grant.name, grant.username, grant.email]
              .filter(Boolean)
              .some((value) =>
                value.toLowerCase().includes(request.search.toLowerCase()),
              )
          : true,
      ),
      page: request.page,
      size: request.size,
      total: accessGrants.length,
    }),
  revokeGatewayAccess: () => Promise.resolve(),
  searchGatewayDirectory: (_gatewayId: string, search: string) =>
    Promise.resolve(
      directoryUsers.filter((user) =>
        [user.name, user.username, user.email]
          .filter(Boolean)
          .some((value) => value.toLowerCase().includes(search.toLowerCase())),
      ),
    ),
  deleteOpenShellGatewayServiceAccount: () => Promise.resolve(),
  findGatewayPlacements: () =>
    Promise.resolve({ hasMore: false, items: [placement] }),
  getGatewayPlacementAvailability: (
    selectors: readonly (readonly string[])[],
  ): Promise<GatewayPlacementAvailability> =>
    Promise.resolve({
      localKind: true,
      placements: selectors.map((selector) => ({
        available: true,
        selector,
      })),
    }),
  getGatewayPlacement: () => Promise.resolve(placement),
  getGatewayPlacements: (clusterIds: readonly string[]) =>
    Promise.resolve(clusterIds.map((id) => ({ ...placement, id }))),
  getGateway: () => Promise.resolve(gateway),
  getOpenShellGatewayServiceAccount: (
    _gatewayId: string,
    serviceAccountId: string,
  ) =>
    Promise.resolve({
      connection: {
        accessTokenLifetimeSeconds: 900,
        audience: gateway.oidcAudience,
        clientId: serviceAccounts[0].clientId,
        gatewayEndpoint: gateway.externalDns,
        gatewayName: gateway.name,
        issuer: gateway.oidcIssuer,
        tokenEndpoint: oidcTokenEndpoint,
      },
      serviceAccount:
        serviceAccounts.find((account) => account.id === serviceAccountId) ??
        serviceAccounts[0],
    }),
  listGateways: () =>
    Promise.resolve({ items: [gateway], page: 1, size: 20, total: 1 }),
  listOpenShellGatewayServiceAccounts: (
    _gatewayId: string,
    request: OpenShellGatewayServiceAccountListRequest,
  ) =>
    Promise.resolve({
      capabilities: {
        allowedRoles: ["openshell-admin", "openshell-user"],
        canCreate: true,
        canManageAll: true,
        expirationPolicy: {
          defaultSeconds: 31_536_000,
          maximumSeconds: 31_536_000,
          minimumSeconds: 3_600,
        },
      },
      items: serviceAccounts.filter((account) =>
        request.search
          ? [account.name, account.clientId, account.description]
              .filter(Boolean)
              .some((value) =>
                value.toLowerCase().includes(request.search.toLowerCase()),
              )
          : true,
      ),
      page: request.page,
      size: request.size,
      total: serviceAccounts.length,
    }),
  provisionGateway: () => Promise.resolve(gateway),
  removeGateway: () => Promise.resolve(),
  renameGateway: (_gatewayId: string, name: string) =>
    Promise.resolve({ ...gateway, name }),
  revokeOpenShellGatewayServiceAccount: (
    _gatewayId: string,
    serviceAccountId: string,
  ) =>
    Promise.resolve({
      ...serviceAccounts.find((account) => account.id === serviceAccountId),
      status: "revoked",
    } as OpenShellGatewayServiceAccountRecord),
  updateOpenShellGatewayServiceAccountRole: (
    _gatewayId: string,
    serviceAccountId: string,
    input: { description?: string; role: string },
  ) =>
    Promise.resolve({
      ...serviceAccounts.find((account) => account.id === serviceAccountId),
      description: input.description,
      role: input.role,
    } as OpenShellGatewayServiceAccountRecord),
} as unknown as GatewayOperations;

function GatewayDetailPreview({
  activeTab = "connection",
  gateway: gatewayForStory = gateway,
}: {
  activeTab?: GatewayDetailTab;
  gateway?: GatewayRecord;
}) {
  return (
    <GatewayUiProvider
      currentUser={{ username: "kim-developer" }}
      gateways={gatewayOperations}
      navigation={navigation}
    >
      <GatewayPage
        activeTab={activeTab}
        gateway={gatewayForStory}
        gatewayId={gatewayForStory.id}
      />
    </GatewayUiProvider>
  );
}

const meta = {
  title: "Production/Gateways/Gateway detail",
  component: GatewayPage,
  parameters: {
    layout: "fullscreen",
  },
  render: () => <GatewayDetailPreview />,
} satisfies Meta<typeof GatewayPage>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Connection: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(
      canvas.getByRole("heading", { name: "payments-gateway" }),
    ).toBeVisible();
    await expect(
      canvas.getByRole("tab", { name: "Connection", selected: true }),
    ).toBeVisible();
  },
};

export const Provisioning: Story = {
  render: () => <GatewayDetailPreview gateway={provisioningGateway} />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(
      canvas.getByText("Gateway database and schema are ready."),
    ).toBeVisible();
    await expect(
      canvas.getByText("Waiting for the gateway console endpoint."),
    ).toBeVisible();
  },
};

export const ManageAccess: Story = {
  render: () => <GatewayDetailPreview activeTab="manage-access" />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(
      canvas.getByRole("heading", { name: "Manage access" }),
    ).toBeVisible();
    await expect(canvas.getByText("Kim Developer")).toBeVisible();
    await expect(canvas.getByText("Alex Operator")).toBeVisible();
  },
};

export const AddUsersModal: Story = {
  render: () => <GatewayDetailPreview activeTab="manage-access" />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", { name: "Add users" }),
    );
    await expect(
      canvas.getByRole("dialog", { name: "Add users" }),
    ).toBeVisible();
  },
};

export const ServiceAccounts: Story = {
  render: () => <GatewayDetailPreview activeTab="service-accounts" />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(
      canvas.getByRole("heading", { name: "Service accounts" }),
    ).toBeVisible();
    await expect(canvas.getByText("ci-pipeline")).toBeVisible();
    await expect(canvas.getByText("observability")).toBeVisible();
  },
};

export const CreateServiceAccountModal: Story = {
  render: () => <GatewayDetailPreview activeTab="service-accounts" />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", { name: "Create service account" }),
    );
    await expect(
      canvas.getByRole("dialog", { name: "Create service account" }),
    ).toBeVisible();
  },
};

export const ServiceAccountSetupModal: Story = {
  render: () => <GatewayDetailPreview activeTab="service-accounts" />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", { name: "Actions for ci-pipeline" }),
    );
    await userEvent.click(
      await canvas.findByRole("menuitem", { name: "View setup instructions" }),
    );
    await expect(
      canvas.getByRole("dialog", { name: "Set up ci-pipeline" }),
    ).toBeVisible();
  },
};

export const EditServiceAccountModal: Story = {
  render: () => <GatewayDetailPreview activeTab="service-accounts" />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", { name: "Actions for ci-pipeline" }),
    );
    await userEvent.click(
      await canvas.findByRole("menuitem", { name: "Edit role" }),
    );
    await expect(
      canvas.getByRole("dialog", { name: "Edit ci-pipeline" }),
    ).toBeVisible();
  },
};

export const RevokeServiceAccountModal: Story = {
  render: () => <GatewayDetailPreview activeTab="service-accounts" />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", { name: "Actions for ci-pipeline" }),
    );
    await userEvent.click(
      await canvas.findByRole("menuitem", { name: "Revoke service account" }),
    );
    await expect(
      canvas.getByRole("dialog", { name: "Revoke ci-pipeline?" }),
    ).toBeVisible();
  },
};

export const DeleteServiceAccountModal: Story = {
  render: () => <GatewayDetailPreview activeTab="service-accounts" />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", { name: "Actions for ci-pipeline" }),
    );
    await userEvent.click(
      await canvas.findByRole("menuitem", { name: "Delete service account" }),
    );
    await expect(
      canvas.getByRole("dialog", { name: "Delete ci-pipeline?" }),
    ).toBeVisible();
  },
};

export const ChangeOwnAccessRoleModal: Story = {
  render: () => <GatewayDetailPreview activeTab="manage-access" />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", { name: "Change role: Kim Developer" }),
    );
    await userEvent.click(await canvas.findByRole("option", { name: "Admin" }));
    await expect(
      canvas.getByRole("dialog", { name: "Change your own role?" }),
    ).toBeVisible();
  },
};

export const RemoveAccessModal: Story = {
  render: () => <GatewayDetailPreview activeTab="manage-access" />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", {
        name: "Remove access: Jordan Automation",
      }),
    );
    await expect(
      canvas.getByRole("dialog", {
        name: "Remove access for Jordan Automation?",
      }),
    ).toBeVisible();
  },
};

export const RenameGatewayModal: Story = {
  render: () => <GatewayDetailPreview />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", { name: "Actions" }),
    );
    await userEvent.click(
      await canvas.findByRole("menuitem", { name: "Rename gateway" }),
    );
    await expect(
      canvas.getByRole("dialog", { name: "Rename gateway" }),
    ).toBeVisible();
  },
};

export const DeleteGatewayModal: Story = {
  render: () => <GatewayDetailPreview />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", { name: "Actions" }),
    );
    await userEvent.click(
      await canvas.findByRole("menuitem", { name: "Delete gateway" }),
    );
    await expect(
      canvas.getByRole("dialog", { name: "Delete payments-gateway?" }),
    ).toBeVisible();
    await expect(canvas.getByText("3 active sandboxes")).toBeVisible();
  },
};

export const Details: Story = {
  render: () => <GatewayDetailPreview activeTab="details" />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(
      canvas.getByRole("tab", { name: "Details", selected: true }),
    ).toBeVisible();
    await expect(canvas.getByText("1.8.0")).toBeVisible();
    await expect(canvas.getByText("aws-us-east-1-production")).toBeVisible();
  },
};
