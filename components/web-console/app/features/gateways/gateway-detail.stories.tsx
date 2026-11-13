import type {
  GatewayOperations,
  GatewayRecord,
  GatewayUiNavigation,
} from "@openshift-online/hypershell-gateway-management-ui";
import {
  GatewayPage,
  GatewayUiProvider,
} from "@openshift-online/hypershell-gateway-management-ui";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { IntlProvider } from "react-intl";

const gateway: GatewayRecord = {
  activeSandboxCount: 3,
  canDelete: true,
  canEdit: true,
  clusterId: "cluster-ibm-public",
  clusterType: "IBM Public",
  consoleUrl: "https://payments-gateway.example.com/console",
  createdAt: "2026-10-29T09:41:00-05:00",
  createdBy: "kim-developer",
  externalDns: "https://payments-gateway.example.com",
  gatewayVersion: "1.8.0",
  id: "payments-gateway",
  name: "payments-gateway",
  namespace: "openshell-payments",
  oidcAudience: "openshell-cli",
  oidcClientId: "openshell-cli",
  oidcIssuer: "https://sso.example.com/realms/openshell",
  phase: "Running",
  provisioningConditions: [
    {
      conditionStatus: "Complete",
      message: "",
      type: "EnvironmentReady",
    },
    {
      conditionStatus: "Complete",
      message: "",
      type: "GatewayHealthy",
    },
  ],
  status: "Healthy",
};

const gatewayOperations = {
  getGatewayPlacement: () =>
    Promise.resolve({
      id: "cluster-ibm-public",
      name: "Cluster East",
      status: "Ready",
    }),
  listGatewayAccess: () =>
    Promise.resolve({
      capabilities: {
        canManageAccess: true,
        canManageOwners: true,
      },
      items: [],
      page: 1,
      size: 20,
      total: 0,
    }),
  listOpenShellGatewayServiceAccounts: () =>
    Promise.resolve({
      capabilities: {
        allowedRoles: ["openshell-admin", "openshell-user"],
        canCreate: true,
        canManageAll: true,
        expirationPolicy: {
          defaultSeconds: 7776000,
          maximumSeconds: 31536000,
          minimumSeconds: 3600,
        },
      },
      items: [],
      page: 1,
      size: 20,
      total: 0,
    }),
} as unknown as GatewayOperations;

const navigation: GatewayUiNavigation = {
  collectionHref: "/gateways",
  createHref: "/gateways/new",
  detailHref: (gatewayId) => `/gateways/${gatewayId}`,
  navigate: () => undefined,
};

function GatewayDetailProductionPreview({
  activeTab,
}: {
  activeTab: "details" | "connection" | "user-access" | "service-accounts";
}) {
  return (
    <IntlProvider locale="en">
      <GatewayUiProvider gateways={gatewayOperations} navigation={navigation}>
        <GatewayPage
          activeTab={activeTab}
          gateway={gateway}
          gatewayId={gateway.id}
        />
      </GatewayUiProvider>
    </IntlProvider>
  );
}

const meta = {
  title: "Production/Gateways/Gateway detail",
  component: GatewayDetailProductionPreview,
  parameters: {
    layout: "fullscreen",
  },
  render: ({ activeTab }) => (
    <GatewayDetailProductionPreview activeTab={activeTab} />
  ),
} satisfies Meta<typeof GatewayDetailProductionPreview>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Details: Story = {
  args: { activeTab: "details" },
};

export const Connection: Story = {
  args: { activeTab: "connection" },
};

export const UserAccess: Story = {
  args: { activeTab: "user-access" },
};

export const ServiceAccounts: Story = {
  args: { activeTab: "service-accounts" },
};
