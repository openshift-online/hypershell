import "@patternfly/react-core/dist/styles/base.css";

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

const preview: Preview = {
  decorators: [
    (Story) => (
      <IntlProvider locale="en" messages={englishMessages}>
        <QueryClientProvider client={queryClient}>
          <Story />
        </QueryClientProvider>
      </IntlProvider>
    ),
  ],
  parameters: {
    a11y: {
      test: "error",
    },
  },
};

export default preview;
