import { reactRouter } from "@react-router/dev/vite";
import { defineConfig } from "vite";

const getApiOrigin = (): string => {
  const configuredOrigin =
    process.env.WEB_CONSOLE_API_ORIGIN ?? "http://127.0.0.1:8000";
  const origin = new URL(configuredOrigin);
  const isLoopback = ["127.0.0.1", "::1", "localhost"].includes(
    origin.hostname,
  );

  if (
    !["http:", "https:"].includes(origin.protocol) ||
    origin.username ||
    origin.password
  ) {
    throw new Error(
      "WEB_CONSOLE_API_ORIGIN must be an HTTP(S) origin without credentials",
    );
  }
  if (!isLoopback) {
    throw new Error(
      "WEB_CONSOLE_API_ORIGIN must use a loopback host for no-auth development",
    );
  }

  return origin.origin;
};

export default defineConfig({
  envDir: false,
  plugins: process.env.STORYBOOK === "true" ? [] : [reactRouter()],
  build: {
    sourcemap: false,
    target: "es2022",
  },
  server: {
    host: process.env.DEV_SERVER_HOST ?? "127.0.0.1",
    port: 5173,
    strictPort: true,
    proxy: {
      "/api": {
        target: getApiOrigin(),
        changeOrigin: false,
      },
    },
  },
  ssr: {
    noExternal: [/^@patternfly\//],
  },
});
