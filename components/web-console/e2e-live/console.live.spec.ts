import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";

import {
  expect,
  test,
  type APIRequestContext,
  type BrowserContext,
  type Page,
} from "@playwright/test";

const username = process.env.E2E_OIDC_USERNAME ?? "admin";
const password = process.env.E2E_OIDC_PASSWORD ?? "admin";
const mode = process.env.E2E_MODE ?? "long";
const provisionTimeout =
  Number(process.env.E2E_PROVISION_TIMEOUT ?? "300") * 1_000;
const consoleTimeout =
  Number(process.env.E2E_CONSOLE_READY_TIMEOUT ?? "300") * 1_000;
const gatewayVersionTimeout =
  Number(process.env.E2E_GATEWAY_VERSION_TIMEOUT ?? "300") * 1_000;
const pageTimeout = Number(process.env.E2E_BROWSER_PAGE_TIMEOUT_MS ?? "90000");
const keepGateway = process.env.E2E_SKIP_CLEANUP === "1";
const runId = Math.random().toString(16).slice(2, 10);
const gatewayName = process.env.E2E_GATEWAY_NAME ?? `e2e-console-gw-${runId}`;
const placementName =
  process.env.E2E_SEED_CLUSTER_NAME ??
  (process.env.E2E_INFRA_DRIVER === "openshift"
    ? "local-openshift"
    : "local-kind");

interface Gateway {
  readonly id: string;
  readonly name: string;
  readonly namespace?: string;
  readonly phase?: string;
  readonly status?: string;
  readonly console_address?: string;
  readonly gateway_version?: string;
}

async function logIn(page: Page) {
  await page.goto("/", { waitUntil: "domcontentloaded" });
  const usernameInput = page.getByLabel(/username/iu);
  const homepageHeading = page.getByRole("heading", {
    level: 2,
    name: "Pick a gateway. Ship your sandboxes.",
  });
  await expect(usernameInput.or(homepageHeading)).toBeVisible({
    timeout: pageTimeout,
  });
  if (await usernameInput.isVisible()) {
    await usernameInput.fill(username);
    await page.getByLabel("Password", { exact: true }).fill(password);
    await page.getByRole("button", { name: /sign in|log in/iu }).click();
  }
  await expect(homepageHeading).toBeVisible({ timeout: pageTimeout });
}

async function readGateway(api: APIRequestContext, id: string) {
  const response = await api.get(`/api/hypershell/v1/gateways/${id}`);
  const body = await response.text();
  expect(response.status(), `gateway read failed: ${body}`).toBe(200);
  return JSON.parse(body) as Gateway;
}

async function requirePlacementAvailability(api: APIRequestContext) {
  const response = await api.post(
    "/api/hypershell/v1/gateways/placement-availability",
    { data: { selectors: [["provider/aws"]] } },
  );
  const body = await response.text();
  expect(
    response.status(),
    `placement availability must be provided by the deployed API: ${body}`,
  ).toBe(200);
}

test.describe("live console user journey", () => {
  test.describe.configure({ mode: "serial" });

  let gatewayId: string | undefined;
  let gateway: Gateway | undefined;
  let authenticatedState:
    Awaited<ReturnType<BrowserContext["storageState"]>> | undefined;

  test.afterAll(async ({ browser }) => {
    if (keepGateway || gatewayId === undefined) return;
    const context = await browser.newContext({
      baseURL:
        process.env.E2E_CONSOLE_URL ?? "https://console.hypershell.localhost",
      ignoreHTTPSErrors: true,
      storageState: authenticatedState,
    });
    const cleanupPage = await context.newPage();
    try {
      if (authenticatedState === undefined) {
        await logIn(cleanupPage);
      }
      const response = await cleanupPage.request.delete(
        `/api/hypershell/v1/gateways/${gatewayId}`,
      );
      expect(
        [200, 202, 204, 404],
        `gateway cleanup failed with HTTP ${String(response.status())}`,
      ).toContain(response.status());
    } finally {
      await context.close();
    }
  });

  test("logs in through Keycloak", async ({ page }) => {
    await logIn(page);
    const session = await page.request.get("/auth/session");
    expect(session.ok()).toBe(true);
    await expect(session.json()).resolves.toMatchObject({
      authenticated: true,
      user: { preferred_username: username },
    });
  });

  test("validates required provisioning fields", async ({ page }) => {
    test.skip(mode !== "long", "extended form validation runs in long mode");
    await logIn(page);
    await page.getByRole("link", { name: "Provision gateway" }).click();
    await page.getByRole("button", { name: "Provision gateway" }).click();
    await expect(
      page.getByText("This field is required.").first(),
    ).toBeVisible();
  });

  test("provisions a gateway", async ({ page }) => {
    test.setTimeout(provisionTimeout + 30_000);
    await logIn(page);
    authenticatedState = await page.context().storageState();
    await page.goto("/gateways/new");
    await requirePlacementAvailability(page.request);
    await page.getByLabel("Gateway name").fill(gatewayName);
    await page.getByRole("radio", { name: `Use ${placementName}` }).check();
    const createResponse = page.waitForResponse(
      (response) =>
        response.request().method() === "POST" &&
        response.url().endsWith("/api/hypershell/v1/gateways"),
    );
    await Promise.all([
      page.waitForURL((url) => {
        const match = /^\/gateways\/([^/]+)$/u.exec(url.pathname);
        return match !== null && match[1] !== "new";
      }),
      page.getByRole("button", { name: "Provision gateway" }).click(),
    ]);
    const createdGateway = (await (await createResponse).json()) as Gateway;
    gatewayId ??= createdGateway.id;
    expect(gatewayId).toBeTruthy();
    await expect(
      page.getByRole("heading", { level: 1, name: gatewayName }),
    ).toBeVisible();
  });

  test("waits for the gateway to become ready", async ({ page }) => {
    test.setTimeout(provisionTimeout + 30_000);
    expect(
      gatewayId,
      "the provisioning test must create a gateway",
    ).toBeDefined();
    if (gatewayId === undefined) return;
    const createdGatewayId = gatewayId;
    await logIn(page);
    await expect
      .poll(
        async () => {
          gateway = await readGateway(page.request, createdGatewayId);
          return gateway.phase;
        },
        { timeout: provisionTimeout, intervals: [2_000, 5_000] },
      )
      .toBe("Running");
    await page.goto(`/gateways/${createdGatewayId}`);
    await expect(page.getByText(gateway?.status ?? "Running")).toBeVisible({
      timeout: pageTimeout,
    });
  });

  test("opens the published OpenShell console", async ({ page }) => {
    test.setTimeout(consoleTimeout + 60_000);
    expect(
      gatewayId,
      "the provisioning test must create a gateway",
    ).toBeDefined();
    if (gatewayId === undefined) return;
    const createdGatewayId = gatewayId;
    await logIn(page);
    await expect
      .poll(
        async () => {
          gateway = await readGateway(page.request, createdGatewayId);
          return gateway.console_address ?? "";
        },
        { timeout: consoleTimeout, intervals: [2_000, 5_000] },
      )
      .not.toBe("");

    await page.goto(`/gateways/${gatewayId}`);
    const consoleAddress = gateway?.console_address ?? "";
    await expect(
      page.getByRole("link", {
        name: `Open console for ${gatewayName} in a new tab`,
      }),
    ).toHaveAttribute("href", consoleAddress, { timeout: pageTimeout });

    const consolePage = await page.context().newPage();
    await consolePage.goto(consoleAddress);
    const login = consolePage.getByLabel(/username/iu);
    if (await login.isVisible()) {
      await login.fill(username);
      await consolePage.getByLabel("Password", { exact: true }).fill(password);
      await consolePage
        .getByRole("button", { name: /sign in|log in/iu })
        .click();
    }
    await expect(
      consolePage.getByText("Cannot reach the OpenShell gateway"),
    ).toBeHidden();
    await consolePage.goto(`${consoleAddress}/workspaces`);
    await expect(
      consolePage.getByRole("heading", { name: /workspaces/iu }),
    ).toBeVisible();
    await consolePage.close();
  });

  test("shows placement and CLI installation choices", async ({ page }) => {
    test.setTimeout(gatewayVersionTimeout + pageTimeout + 30_000);
    expect(
      gatewayId,
      "the provisioning test must create a gateway",
    ).toBeDefined();
    if (gatewayId === undefined) return;
    const createdGatewayId = gatewayId;
    await logIn(page);
    await page.goto("/gateways/new");
    await expect(
      page.getByRole("radio", { name: `Use ${placementName}` }),
    ).toBeVisible();
    await page.goto(`/gateways/${createdGatewayId}`);
    await expect
      .poll(
        async () => {
          gateway = await readGateway(page.request, createdGatewayId);
          return gateway.gateway_version ?? "";
        },
        {
          timeout: gatewayVersionTimeout,
          intervals: [2_000, 5_000],
        },
      )
      .not.toBe("");
    const docs = page.getByRole("link", {
      name: /^View installation documentation/iu,
    });
    await expect(docs).toHaveAttribute("href", /^https:\/\//u, {
      timeout: pageTimeout,
    });
    await expect(docs).toHaveAttribute("target", "_blank");
    await expect(docs).toHaveAttribute("rel", /noopener/u);

    // E2E-COV-05.01/05.04: the console must render the REAL, version-derived
    // install command, not a pinned constant. Scrape the exact text the UI hands
    // the operator by reading the copy button's clipboard payload, then assert it
    // carries the live reconciled gateway_version. This reads the actual rendered
    // command, so UI drift (unlike a Go-to-Go comparison) is caught here.
    await page
      .context()
      .grantPermissions(["clipboard-read", "clipboard-write"]);
    const installVersion = gateway?.gateway_version ?? "";
    expect(installVersion, "gateway reported a reconciled version").not.toBe(
      "",
    );
    const expectedInstallerVersion = installVersion.startsWith("v")
      ? installVersion
      : `v${installVersion}`;
    const copyInstall = page.getByRole("button", {
      name: "Copy the OpenShell installation command",
    });
    await expect(copyInstall).toBeVisible({ timeout: pageTimeout });
    await copyInstall.click();
    const renderedInstall = await page.evaluate(() =>
      navigator.clipboard.readText(),
    );
    // Exact equality, not substrings: the live rendered command must equal the
    // shared fixture (the same one the Go merge-gate phase asserts against) with
    // the live installer version substituted. A wrong URL, flag, quoting, or line
    // would fail this.
    const fixture = readFileSync(
      fileURLToPath(
        new URL(
          "../../../tests/e2e/testdata/ui-install-command.golden",
          import.meta.url,
        ),
      ),
      "utf8",
    ).replace(/\n$/u, "");
    const expectedCommand = fixture.replaceAll(
      "__OPENSHELL_INSTALLER_VERSION__",
      expectedInstallerVersion,
    );
    expect(renderedInstall).toBe(expectedCommand);
  });

  test("deletes the gateway from its actions menu", async ({ page }) => {
    test.skip(keepGateway, "E2E_SKIP_CLEANUP=1 preserves the gateway");
    expect(
      gatewayId,
      "the provisioning test must create a gateway",
    ).toBeDefined();
    if (gatewayId === undefined) return;
    const createdGatewayId = gatewayId;
    await logIn(page);
    await page.goto(`/gateways/${createdGatewayId}`);
    await page.getByRole("button", { name: "Actions" }).click();
    await page.getByRole("menuitem", { name: "Delete gateway" }).click();
    const dialog = page.getByRole("dialog", {
      name: `Delete ${gatewayName}?`,
    });
    await dialog.getByRole("button", { name: "Delete gateway" }).click();
    await expect(page.getByText(`Gateway ${gatewayName} deleted`)).toBeVisible({
      timeout: pageTimeout,
    });
    await expect
      .poll(
        async () =>
          (
            await page.request.get(
              `/api/hypershell/v1/gateways/${createdGatewayId}`,
            )
          ).status(),
        { timeout: 60_000, intervals: [2_000, 5_000] },
      )
      .toBe(404);
    gatewayId = undefined;
  });
});
