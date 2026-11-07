import type {
  GatewayConnection,
  GatewayOperations,
  GatewayUiNavigation,
} from "@openshift-online/hypershell-gateway-management-ui";
import {
  GatewaysPage,
  GatewayUiProvider,
} from "@openshift-online/hypershell-gateway-management-ui";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { IntlProvider } from "react-intl";

const gateways: readonly GatewayConnection[] = [
  {
    activeSandboxCount: 3,
    canDelete: true,
    clusterId: "cluster-ibm-public",
    clusterName: "Cluster East",
    consoleUrl: "https://console.dev.example.com",
    createdAt: "2026-10-29T09:41:00-05:00",
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
    consoleUrl: "https://console.prod.example.com",
    createdAt: "2026-10-28T16:12:00-05:00",
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
    clusterId: "cluster-ibm-kata",
    clusterName: "Cluster West",
    consoleUrl: "https://console.staging.example.com",
    createdAt: "2026-09-22T11:03:00-05:00",
    endpoint: "https://gw-openshell-9f4b40e1c471c047.gw.localhost:443",
    gatewayVersion: "1.7.4",
    id: "integrations-gateway",
    name: "integrations-gateway",
    phase: "Running",
    status: "Degraded",
  },
];

const gatewayOperations = {} as GatewayOperations;

const navigation: GatewayUiNavigation = {
  collectionHref: "/gateways",
  createHref: "/gateways/new",
  detailHref: (gatewayId) => `/gateways/${gatewayId}`,
  navigate: () => undefined,
};

function GatewayListProductionPreview() {
  return (
    <IntlProvider locale="en">
      <GatewayUiProvider gateways={gatewayOperations} navigation={navigation}>
        <GatewaysPage gateways={gateways} onRefresh={() => undefined} />
      </GatewayUiProvider>
    </IntlProvider>
  );
}

const meta = {
  title: "Production/Gateways/Gateway list",
  component: GatewaysPage,
  parameters: {
    layout: "fullscreen",
  },
  render: () => <GatewayListProductionPreview />,
} satisfies Meta<typeof GatewaysPage>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Loaded: Story = {};
