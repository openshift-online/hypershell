import "@patternfly/react-core/dist/styles/base.css";
import "@patternfly/react-styles/css/utilities/_index.css";

import { useEffect, type ReactNode } from "react";
import type { Preview } from "@storybook/react-vite";

const ColorSchemeDecorator = ({
  children,
  scheme,
}: {
  children: ReactNode;
  scheme: "light" | "dark";
}) => {
  useEffect(() => {
    document.documentElement.classList.toggle("pf-v6-theme-dark", scheme === "dark");
    document.documentElement.style.colorScheme = scheme;

    return () => {
      document.documentElement.classList.remove("pf-v6-theme-dark");
      document.documentElement.style.removeProperty("color-scheme");
    };
  }, [scheme]);

  return children;
};

const preview: Preview = {
  globalTypes: {
    colorScheme: {
      description: "Color scheme",
      defaultValue: "light",
      toolbar: {
        icon: "mirror",
        items: [
          { value: "light", title: "Light" },
          { value: "dark", title: "Dark" },
        ],
      },
    },
  },
  decorators: [
    (Story, context) => (
      <ColorSchemeDecorator scheme={context.globals.colorScheme}>
        <Story />
      </ColorSchemeDecorator>
    ),
  ],
  parameters: {
    layout: "fullscreen",
  },
};

export default preview;
