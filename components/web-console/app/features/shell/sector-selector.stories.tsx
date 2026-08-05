import type { Meta, StoryObj } from "@storybook/react-vite";
import { MemoryRouter } from "react-router";

import { SectorSelector } from "./sector-selector";

const edgeEast = { id: "edge-east", name: "Edge East" };
const sectors = [
  edgeEast,
  { id: "edge-west", name: "Edge West" },
  { id: "platform-services", name: "Platform services" },
];

const meta: Meta<typeof SectorSelector> = {
  title: "Application/Sector selector",
  component: SectorSelector,
  decorators: [
    (StoryComponent) => (
      <MemoryRouter>
        <StoryComponent />
      </MemoryRouter>
    ),
  ],
  args: {
    onSelectSector: () => undefined,
    sectors,
    selectedSectorId: "edge-east",
  },
};

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};

export const Loading: Story = {
  args: {
    isLoading: true,
    sectors: [edgeEast],
  },
};

export const Error: Story = {
  args: {
    hasError: true,
    sectors: [edgeEast],
  },
};

export const LongName: Story = {
  args: {
    sectors: [
      {
        id: "global-platform-services",
        name: "Global platform services and shared gateway infrastructure",
      },
    ],
    selectedSectorId: "global-platform-services",
  },
};
