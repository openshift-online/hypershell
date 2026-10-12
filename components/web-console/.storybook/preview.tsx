import "@patternfly/react-core/dist/styles/base.css";
import "@patternfly/react-styles/css/utilities/_index.css";

import { useEffect, type ReactNode } from "react";
import type { Preview } from "@storybook/react-vite";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { IntlProvider } from "react-intl";

import { englishMessages } from "../app/i18n/catalog";

const queryClient = new QueryClient({
  defaultOptions: {
    queries: { retry: false },
    mutations: { retry: false },
  },
});

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
        <IntlProvider locale="en" messages={englishMessages}>
          <QueryClientProvider client={queryClient}>
            <Story />
          </QueryClientProvider>
        </IntlProvider>
      </ColorSchemeDecorator>
    ),
  ],
  parameters: {
    a11y: {
      test: "error",
    },
  },
};

export default preview;
