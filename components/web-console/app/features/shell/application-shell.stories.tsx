import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  GatewayPage,
  GatewaysPage,
  type GatewayConnection,
  type GatewayRecord,
} from "@openshift-online/hypershell-gateway-management-ui";
import { IntlProvider } from "react-intl";
import { MemoryRouter, Route, Routes } from "react-router";

import { englishMessages } from "../../i18n/catalog";
import { ApplicationShell } from "./application-shell";

const previewGateway: GatewayConnection = {
  clusterName: "Hub cluster",
  consoleUrl: "https://console.example.test",
  endpoint: "https://gateway.example.test:443",
  id: "gateway-b",
  name: "OpenShell gateway",
  oidcAudience: "openshell-cli",
  oidcClientId: "openshell-cli",
  oidcIssuer: "https://issuer.example.test",
  status: "Ready",
};

const previewGatewayResource: GatewayRecord = {
  clusterId: "",
  databaseId: "database-1",
  externalDns: "gateway.example.test",
  id: "gateway-b",
  name: "OpenShell gateway",
  namespace: "openshell",
  phase: "",
  releaseId: "release-1",
  status: "Ready",
};

const pseudoMessages = Object.fromEntries(
  Object.entries(englishMessages).map(([id, message]) => [
    id,
    `［${message.replaceAll("a", "à").replaceAll("e", "ë")}］`,
  ]),
);

function ShellPreview({ initialPath = "/" }: { initialPath?: string }) {
  return (
    <MemoryRouter initialEntries={[initialPath]}>
      <Routes>
        <Route element={<ApplicationShell />}>
          <Route
            path="/"
            element={<GatewaysPage gateways={[previewGateway]} />}
          />
          <Route
            path="gateways/:gatewayId"
            element={
              <GatewayPage
                gateway={previewGatewayResource}
                gatewayId="gateway-b"
              />
            }
          />
        </Route>
      </Routes>
    </MemoryRouter>
  );
}

const meta = {
  title: "HyperShell/Shell",
  component: ApplicationShell,
  parameters: {
    layout: "fullscreen",
  },
  render: () => <ShellPreview />,
} satisfies Meta<typeof ApplicationShell>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Gateways: Story = {};

export const GatewayDetails: Story = {
  render: () => <ShellPreview initialPath="/gateways/gateway-b" />,
};

export const PseudoLocalized: Story = {
  decorators: [
    (StoryComponent) => (
      <IntlProvider locale="en-XA" messages={pseudoMessages}>
        <StoryComponent />
      </IntlProvider>
    ),
  ],
};

export const RightToLeft: Story = {
  decorators: [
    (StoryComponent) => (
      <div dir="rtl" lang="ar">
        <IntlProvider locale="ar" messages={englishMessages}>
          <StoryComponent />
        </IntlProvider>
      </div>
    ),
  ],
};
