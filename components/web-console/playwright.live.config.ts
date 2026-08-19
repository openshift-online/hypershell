import { defineConfig, devices } from "@playwright/test";

// Live-cluster e2e configuration. Unlike playwright.config.ts (which serves a
// mocked dev build), this suite runs against a deployed HyperShell environment
// -- a Kind cluster in CI or a developer's local cluster -- and asserts real
// distributed traces reach Jaeger. There is no webServer: the console is the
// deployed BFF, reached over its self-signed gateway TLS.
const consoleUrl =
  process.env.E2E_CONSOLE_URL ?? "https://console.hypershell.localhost";

export default defineConfig({
  testDir: "./e2e-live",
  fullyParallel: false,
  forbidOnly: Boolean(process.env.CI),
  retries: process.env.CI ? 2 : 0,
  reporter: process.env.CI ? "github" : "list",
  // Trace export is asynchronous (batch span processor timer plus flush on page
  // hide), so a single test polls Jaeger and needs a generous ceiling.
  timeout: 120_000,
  use: {
    baseURL: consoleUrl,
    // The gateway serves a Kind self-signed certificate for
    // *.hypershell.localhost; trust it the same way the bash e2e curl -k does.
    ignoreHTTPSErrors: true,
    trace: "on-first-retry",
  },
  projects: [{ name: "chromium", use: { ...devices["Desktop Chrome"] } }],
});
