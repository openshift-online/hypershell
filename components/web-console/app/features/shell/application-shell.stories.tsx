import type { Meta, StoryObj } from "@storybook/react-vite";
import { IntlProvider } from "react-intl";
import { MemoryRouter, Route, Routes } from "react-router";

import { englishMessages } from "../../i18n/catalog";
import { previewGateways } from "../gateways/gateway-connections";
import { AdminShell } from "./application-shell";
import { AdminGatewayPage, AdminGatewaysPage } from "./shell-pages";

const pseudoMessages = Object.fromEntries(
  Object.entries(englishMessages).map(([id, message]) => [
    id,
    `［${message.replaceAll("a", "à").replaceAll("e", "ë")}］`,
  ]),
);

function ShellPreview({ initialPath = "/admin" }: { initialPath?: string }) {
  return (
    <MemoryRouter initialEntries={[initialPath]}>
      <Routes>
        <Route element={<AdminShell />}>
          <Route
            path="admin"
            element={<AdminGatewaysPage gateways={previewGateways} />}
          />
          <Route
            path="admin/gateways/:gatewayId"
            element={<AdminGatewayPage />}
          />
        </Route>
      </Routes>
    </MemoryRouter>
  );
}

const meta = {
  title: "Administration/Shell",
  component: AdminShell,
  parameters: {
    layout: "fullscreen",
  },
  render: () => <ShellPreview />,
} satisfies Meta<typeof AdminShell>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Gateways: Story = {};

export const GatewayDetails: Story = {
  render: () => <ShellPreview initialPath="/admin/gateways/gateway-b" />,
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
