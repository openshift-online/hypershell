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
const devUsername = process.env.E2E_DEV_USERNAME ?? "developer";
const directoryTimeout =
  Number(process.env.E2E_DIRECTORY_TIMEOUT ?? "420") * 1_000;
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
// Radio label suffix for the local development placement option. The Kind
// option is offered through the provider/kind availability check and is titled
// "Use Kind" (WEB-PLC-05); it is no longer named after the seed cluster.
const placementName =
  process.env.E2E_SEED_CLUSTER_NAME ??
  (process.env.E2E_INFRA_DRIVER === "openshift" ? "local-openshift" : "Kind");

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

async function openAccessTab(page: Page, id: string) {
  await page.goto(`/gateways/${id}`);
  await page.getByRole("tab", { name: "User access" }).click();
  await expect(page.getByRole("button", { name: "Add users" })).toBeVisible({
    timeout: pageTimeout,
  });
}

function accessRow(page: Page, user: string) {
  return page
    .getByRole("row")
    .filter({ has: page.getByRole("gridcell", { name: user, exact: true }) });
}

// The picker reads a control-plane snapshot of the realm that refreshes on a
// timer (GTW-ACC-09), so wait until the seeded user is searchable.
async function waitForDirectoryUser(
  api: APIRequestContext,
  id: string,
  user: string,
) {
  await expect
    .poll(
      async () => {
        const response = await api.get(
          `/api/hypershell/v1/gateways/${id}/access/directory?search=${encodeURIComponent(user)}`,
        );
        if (!response.ok()) return [];
        const body = (await response.json()) as {
          items?: { username: string }[];
        };
        return (body.items ?? []).map((item) => item.username);
      },
      { timeout: directoryTimeout, intervals: [2_000, 5_000, 10_000] },
    )
    .toContain(user);
}

async function readGateway(api: APIRequestContext, id: string) {
  const response = await api.get(`/api/hypershell/v1/gateways/${id}`);
  const body = await response.text();
  expect(response.status(), `gateway read failed: ${body}`).toBe(200);
  return JSON.parse(body) as Gateway;
}

async function requirePlacementAvailability(api: APIRequestContext) {
  // The journey checks the local development radio on the kind driver and a
  // managed placement otherwise. Assert the matching selector is available so
  // a missing option fails here with a clear message instead of a radio
  // locator timeout.
  const selector =
    process.env.E2E_INFRA_DRIVER === "openshift"
      ? ["provider/aws"]
      : ["provider/kind"];
  const response = await api.post(
    "/api/hypershell/v1/gateways/placement-availability",
    { data: { selectors: [selector] } },
  );
  const body = await response.text();
  expect(
    response.status(),
    `placement availability must be provided by the deployed API: ${body}`,
  ).toBe(200);
  const availability = JSON.parse(body) as {
    placements?: { selector: string[]; available: boolean }[];
  };
  expect(
    availability.placements?.find(
      (placement) =>
        placement.selector.length === selector.length &&
        placement.selector.every((label, index) => label === selector[index]),
    ),
    `placement availability for ${selector.join(",")}: ${body}`,
  ).toMatchObject({ available: true });
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
    await page.getByRole("link", { name: "Create a gateway" }).click();
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
    await expect(
      page
        .getByRole("heading", { level: 1, name: gatewayName })
        .locator("../..")
        .getByText(gateway?.status ?? "Running"),
    ).toBeVisible({ timeout: pageTimeout });
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
    ).toHaveAttribute("href", consoleAddress);

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
    await page.getByRole("tab", { name: "Connection" }).click();
    const docs = page.getByRole("link", {
      name: /^View installation documentation/iu,
    });
    await expect(docs).toHaveAttribute("href", /^https:\/\//u, {
      timeout: pageTimeout,
    });
    await expect(docs).toHaveAttribute("target", "_blank");
    await expect(docs).toHaveAttribute("rel", /noopener/u);
    const installStep = page.getByRole("button", {
      name: /^1 Install the OpenShell CLI/u,
    });
    await installStep.click();
    await expect(installStep).toHaveAttribute("aria-expanded", "true");

    // E2E-COV-05.01/05.04: the console must render the REAL, version-derived install
    // command, not a pinned constant. Scrape the exact text the UI hands the operator
    // by reading the copy button's clipboard payload, then assert it equals the shared
    // fixture (the same file the Go merge-gate phase asserts against) with the live
    // installer version substituted. This reads the actual rendered command, so UI
    // drift (unlike a Go-to-Go comparison) is caught here.
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
      name: "Copy command for step 1, Install the OpenShell CLI",
    });
    await expect(copyInstall).toBeVisible({ timeout: pageTimeout });
    await copyInstall.click();
    const renderedInstall = await page.evaluate(() =>
      navigator.clipboard.readText(),
    );
    const fixture = readFileSync(
      fileURLToPath(
        new URL(
          "../../../packages/gateway-management-ui/src/gateways/ui-install-command.golden",
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

  test("grants a realm user access through Add users", async ({ page }) => {
    // GTW-ACC-03, GTW-ACC-04.01, GTW-ACC-04.02, GTW-ACC-09.01
    test.setTimeout(directoryTimeout + pageTimeout + 60_000);
    expect(
      gatewayId,
      "the provisioning test must create a gateway",
    ).toBeDefined();
    if (gatewayId === undefined) return;
    const createdGatewayId = gatewayId;
    await logIn(page);
    await waitForDirectoryUser(page.request, createdGatewayId, devUsername);
    await openAccessTab(page, createdGatewayId);

    await page.getByRole("button", { name: "Add users" }).click();
    const dialog = page.getByRole("dialog", { name: "Add users" });
    await dialog
      .getByRole("combobox", { name: "Search the identity directory" })
      .fill(devUsername);
    await page
      .getByRole("option", { name: new RegExp(devUsername, "iu") })
      .click();
    await dialog.getByRole("radio", { name: /^User/u }).check();
    const grantResponse = page.waitForResponse(
      (response) =>
        response.request().method() === "POST" &&
        response.url().endsWith(`/gateways/${createdGatewayId}/access`),
    );
    await dialog.getByRole("button", { name: "Grant access" }).click();
    expect((await grantResponse).ok()).toBe(true);

    // A "user" grant keeps the dialog open on the workspace-access step.
    const workspaceStep = page.getByRole("dialog", {
      name: "Grant workspace access",
    });
    await expect(workspaceStep).toBeVisible();
    await workspaceStep.getByRole("button", { name: "Close" }).first().click();
    await expect(workspaceStep).toBeHidden();

    const row = accessRow(page, devUsername);
    await expect(row).toBeVisible({ timeout: pageTimeout });
    await expect(
      row.getByRole("button", { name: /^Change role:/u }),
    ).toHaveText("User");
  });

  test("changes and revokes a granted user", async ({ page }) => {
    // GTW-ACC-05.01, GTW-ACC-06.01
    test.skip(mode !== "long", "role change and revoke run in long mode");
    expect(
      gatewayId,
      "the provisioning test must create a gateway",
    ).toBeDefined();
    if (gatewayId === undefined) return;
    await logIn(page);
    await openAccessTab(page, gatewayId);

    const row = accessRow(page, devUsername);
    await expect(row).toBeVisible({ timeout: pageTimeout });
    const roleControl = row.getByRole("button", { name: /^Change role:/u });
    await roleControl.click();
    await page.getByRole("option", { name: "Admin" }).click();
    await expect(roleControl).toHaveText("Admin", { timeout: pageTimeout });

    await row.getByRole("button", { name: /^Remove access:/u }).click();
    const confirm = page.getByRole("dialog", { name: /^Remove access for/u });
    await confirm.getByRole("button", { name: "Remove access" }).click();
    await expect(confirm).toBeHidden();
    await expect(accessRow(page, devUsername)).toHaveCount(0, {
      timeout: pageTimeout,
    });
  });

  test("shows no results for an unknown directory user", async ({ page }) => {
    // GTW-ACC-04.06, GTW-ACC-09.01: the picker cannot select a missing user.
    test.skip(mode !== "long", "negative picker case runs in long mode");
    expect(
      gatewayId,
      "the provisioning test must create a gateway",
    ).toBeDefined();
    if (gatewayId === undefined) return;
    await logIn(page);
    await openAccessTab(page, gatewayId);

    await page.getByRole("button", { name: "Add users" }).click();
    const dialog = page.getByRole("dialog", { name: "Add users" });
    await dialog
      .getByRole("combobox", { name: "Search the identity directory" })
      .fill(`ghost-${runId}`);
    await expect(page.getByText("No matching users")).toBeVisible({
      timeout: pageTimeout,
    });
    await expect(
      dialog.getByRole("button", { name: "Grant access" }),
    ).toBeDisabled();
    // Close the open results list (it overlaps the footer) before cancelling.
    await dialog.getByRole("heading", { name: "Add users" }).click();
    await dialog.getByRole("button", { name: "Cancel" }).click();
    await expect(dialog).toBeHidden();
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
    await expect
      .poll(
        async () =>
          (
            await page.request.get(
              `/api/hypershell/v1/gateways/${createdGatewayId}`,
            )
          ).status(),
        { timeout: pageTimeout, intervals: [2_000, 5_000] },
      )
      .toBe(200);
    await page.goto(`/gateways/${createdGatewayId}`);
    await expect(
      page.getByRole("heading", { name: gatewayName, exact: true }),
    ).toBeVisible();
    await page.getByRole("button", { name: "Actions" }).click();
    await page.getByRole("menuitem", { name: "Delete gateway" }).click();
    const dialog = page.getByRole("dialog", {
      name: `Delete ${gatewayName}?`,
    });
    await dialog.getByRole("button", { name: "Delete gateway" }).click();
    await expect(
      page.getByText(`Gateway ${gatewayName} deleted`),
    ).toBeVisible();
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
