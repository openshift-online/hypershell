import type { Meta, StoryObj } from "@storybook/react-vite";
import { MemoryRouter, Route, Routes } from "react-router";

import {
  GatewayDetailsPage,
  GatewayDirectoryPage,
} from "../gateways/gateway-directory";
import { UserShell } from "./user-shell";

function ShellPreview({ initialPath = "/" }: { initialPath?: string }) {
  return (
    <MemoryRouter initialEntries={[initialPath]}>
      <Routes>
        <Route element={<UserShell />}>
          <Route index element={<GatewayDirectoryPage />} />
          <Route path="gateways/:gatewayId" element={<GatewayDetailsPage />} />
        </Route>
      </Routes>
    </MemoryRouter>
  );
}

const meta = {
  title: "HyperShell/User shell",
  component: UserShell,
  parameters: {
    layout: "fullscreen",
  },
  render: () => <ShellPreview />,
} satisfies Meta<typeof UserShell>;

export default meta;
type Story = StoryObj<typeof meta>;

export const GatewayDirectory: Story = {};

export const GatewayDetails: Story = {
  render: () => <ShellPreview initialPath="/gateways/openshell-gateway-test" />,
};
