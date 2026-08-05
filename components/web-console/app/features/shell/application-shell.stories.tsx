import type { Meta, StoryObj } from "@storybook/react-vite";
import { IntlProvider } from "react-intl";
import { MemoryRouter, Route, Routes } from "react-router";

import { englishMessages } from "../../i18n/catalog";
import { ApplicationShell } from "./application-shell";
import { GatewayPage, OverviewPage } from "./shell-pages";

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
          <Route index element={<OverviewPage />} />
          <Route
            path="fleets/:fleetId/gateways/:gatewayId"
            element={<GatewayPage />}
          />
        </Route>
      </Routes>
    </MemoryRouter>
  );
}

const meta = {
  title: "Application/Shell",
  component: ApplicationShell,
  parameters: {
    layout: "fullscreen",
  },
  render: () => <ShellPreview />,
} satisfies Meta<typeof ApplicationShell>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Overview: Story = {};

export const SelectedSector: Story = {
  render: () => (
    <ShellPreview initialPath="/fleets/sector-a/gateways/gateway-b" />
  ),
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
