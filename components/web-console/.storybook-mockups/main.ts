import { resolve } from "node:path";

import type { StorybookConfig } from "@storybook/react-vite";

const config: StorybookConfig = {
  stories: ["../../../specs/web-console/mockups/**/*.stories.tsx"],
  framework: {
    name: "@storybook/react-vite",
    options: {},
  },
  viteFinal: (config) => ({
    ...config,
    resolve: {
      ...config.resolve,
      alias: Object.assign({}, config.resolve?.alias, {
        react: resolve(process.cwd(), "node_modules/react"),
        "react-dom": resolve(process.cwd(), "node_modules/react-dom"),
        "@patternfly/react-core": resolve(
          process.cwd(),
          "node_modules/@patternfly/react-core",
        ),
        "@patternfly/react-icons": resolve(
          process.cwd(),
          "node_modules/@patternfly/react-icons",
        ),
        "@patternfly/react-table": resolve(
          process.cwd(),
          "node_modules/@patternfly/react-table",
        ),
      }),
    },
  }),
};

export default config;
