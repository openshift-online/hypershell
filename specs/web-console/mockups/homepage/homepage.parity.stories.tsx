import type { Meta, StoryObj } from "@storybook/react-vite";
import type { HomepageState } from "./homepage";
import { MockupParityFrame } from "../shell/mockup-template";
import { HomepagePageSurface } from "./homepage-page-surface";

const meta = {
  title: "Parity/Homepage",
  component: HomepagePageSurface,
  parameters: { layout: "fullscreen" },
} satisfies Meta<typeof HomepagePageSurface>;

export default meta;
type Story = StoryObj<typeof meta>;

function makeStory(state: HomepageState): Story {
  return {
    args: { state },
    render: (args) => (
      <MockupParityFrame>
        <HomepagePageSurface state={args.state} />
      </MockupParityFrame>
    ),
  };
}

export const Loaded: Story = makeStory("loaded");
export const PartialData: Story = makeStory("partial");
export const ClusterError: Story = makeStory("clusterError");
export const ClusterWarning: Story = makeStory("clusterWarning");
export const NothingNeedsAttention: Story = makeStory("noAttention");
export const PlatformRestricted: Story = makeStory("platformRestricted");
