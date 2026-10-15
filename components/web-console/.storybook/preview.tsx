import "@patternfly/react-core/dist/styles/base.css";
import "@patternfly/react-styles/css/utilities/_index.css";

import { useEffect, useState, type ReactNode } from "react";
import type { Preview } from "@storybook/react-vite";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { IntlProvider } from "react-intl";

import { englishMessages } from "../app/i18n/catalog";

function StorybookQueryProvider({ children }: { children: ReactNode }) {
  const [queryClient] = useState(
    () =>
      new QueryClient({
        defaultOptions: {
          queries: { retry: false },
          mutations: { retry: false },
        },
      }),
  );

  return (
    <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  );
}

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
    document.documentElement.dataset.colorScheme = scheme;
    document.documentElement.style.colorScheme = scheme;
    document.documentElement.style.setProperty(
      "--aws-wordmark-color",
      scheme === "dark" ? "#ffffff" : "#252f3e",
    );

    return () => {
      document.documentElement.classList.remove("pf-v6-theme-dark");
      delete document.documentElement.dataset.colorScheme;
      document.documentElement.style.removeProperty("color-scheme");
      document.documentElement.style.removeProperty("--aws-wordmark-color");
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
      <ColorSchemeDecorator
        scheme={context.globals.colorScheme as "light" | "dark"}
      >
        <IntlProvider locale="en" messages={englishMessages}>
          <StorybookQueryProvider>
            <Story />
          </StorybookQueryProvider>
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
