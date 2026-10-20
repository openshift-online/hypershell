import "@patternfly/react-core/dist/styles/base.css";
import "@patternfly/react-styles/css/utilities/_index.css";

import { useEffect, type ReactNode } from "react";
import type { Preview } from "@storybook/react-vite";

const awsLogoVisibilityStyles =
  ".aws-logo-dark { display: none !important; } html.pf-v6-theme-dark .aws-logo-light { display: none !important; } html.pf-v6-theme-dark .aws-logo-dark { display: block !important; }";

const ColorSchemeDecorator = ({
  children,
  scheme,
}: {
  children: ReactNode;
  scheme: "light" | "dark";
}) => {
  useEffect(() => {
    document.documentElement.classList.toggle(
      "pf-v6-theme-dark",
      scheme === "dark",
    );
    document.documentElement.style.setProperty(
      "--aws-wordmark-color",
      scheme === "dark" ? "#ffffff" : "#252f3e",
    );
    document.documentElement.style.colorScheme = scheme;

    return () => {
      document.documentElement.classList.remove("pf-v6-theme-dark");
      document.documentElement.style.removeProperty("--aws-wordmark-color");
      document.documentElement.style.removeProperty("color-scheme");
    };
  }, [scheme]);

  return (
    <>
      <style dangerouslySetInnerHTML={{ __html: awsLogoVisibilityStyles }} />
      {children}
    </>
  );
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
      <ColorSchemeDecorator
        scheme={context.globals.colorScheme as "light" | "dark"}
      >
        <Story />
      </ColorSchemeDecorator>
    ),
  ],
  parameters: {
    layout: "fullscreen",
  },
};

export default preview;
