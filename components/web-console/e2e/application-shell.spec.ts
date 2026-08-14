import AxeBuilder from "@axe-core/playwright";
import { expect, test } from "@playwright/test";

const gateway = {
  cluster_id: "cluster-east",
  created_at: "2026-08-10T14:30:00Z",
  database_id: "database-1",
  external_dns: "gateway.example.test",
  fleet_id: "",
  href: "/api/hypershell/v1/gateways/openshell-gateway-test",
  id: "openshell-gateway-test",
  kind: "Gateway",
  name: "openshell-gateway-test",
  namespace: "openshell",
  phase: "",
  release_id: "release-1",
  service_type: "",
  status: "Ready",
  tls_mode: "",
  updated_at: null,
};

const managedCluster = {
  api_server_url: "https://api.east.example.test",
  created_at: null,
  fleet_id: "fleet-1",
  href: "/api/hypershell/v1/managed_clusters/cluster-east",
  id: "cluster-east",
  kind: "ManagedCluster",
  kubeconfig_secret: "cluster-east-kubeconfig",
  name: "Cluster East",
  provider: "AWS",
  region: "us-east-1",
  status: "Ready",
  updated_at: null,
};

test.beforeEach(async ({ browserName, page }) => {
  let gatewayDeleted = false;
  let gatewayName = gateway.name;
  if (browserName !== "chromium") {
    await page.addInitScript(() => {
      let clipboardText = "";
      Object.defineProperty(navigator, "clipboard", {
        configurable: true,
        value: {
          readText: () => Promise.resolve(clipboardText),
          writeText: (text: string) => {
            clipboardText = text;
            return Promise.resolve();
          },
        },
      });
    });
  }

  await page.route("**/api/hypershell/v1/gateways**", async (route) => {
    const request = route.request();
    if (request.method() === "DELETE") {
      gatewayDeleted = true;
      await route.fulfill({ status: 204 });
      return;
    }
    if (request.method() === "PATCH") {
      const patch = request.postDataJSON() as { name?: unknown };
      if (typeof patch.name === "string") {
        gatewayName = patch.name;
      }
      await route.fulfill({
        body: JSON.stringify({ ...gateway, name: gatewayName }),
        contentType: "application/json",
        status: 200,
      });
      return;
    }
    if (request.method() !== "GET") {
      await route.continue();
      return;
    }

    const gatewayId = new URL(request.url()).pathname.split("/").at(-1);
    const body =
      gatewayId === "gateways"
        ? {
            items: gatewayDeleted ? [] : [{ ...gateway, name: gatewayName }],
            kind: "GatewayList",
            page: 1,
            size: 100,
            total: gatewayDeleted ? 0 : 1,
          }
        : { ...gateway, name: gatewayName };
    await route.fulfill({
      body: JSON.stringify(body),
      contentType: "application/json",
      status: 200,
    });
  });
  await page.route("**/api/hypershell/v1/managed_clusters**", async (route) => {
    const isCollection = new URL(route.request().url()).pathname.endsWith(
      "/managed_clusters",
    );
    await route.fulfill({
      body: JSON.stringify(
        isCollection
          ? {
              items: [managedCluster],
              kind: "ManagedClusterList",
              page: 1,
              size: 1,
              total: 1,
            }
          : managedCluster,
      ),
      contentType: "application/json",
      status: 200,
    });
  });
});

test("makes gateway management the primary HyperShell experience", async ({
  page,
}) => {
  await page.goto("/");

  await expect(page).toHaveTitle("HyperShell - OpenShell Gateways");
  await expect(
    page.getByRole("link", { name: "HyperShell", exact: true }),
  ).toBeVisible();
  await expect(page.getByText("Administration")).toHaveCount(0);
  await expect(
    page.getByText("Provision and manage OpenShell gateways."),
  ).toHaveCount(0);
  await expect(
    page.getByRole("heading", { level: 1, name: "OpenShell Gateways" }),
  ).toBeVisible();
  await expect(
    page.getByRole("link", { name: "Provision gateway" }),
  ).toBeVisible();
  await expect(
    page
      .getByRole("grid", { name: "OpenShell Gateways" })
      .getByText("Cluster East"),
  ).toBeVisible();
  const gatewayGrid = page.getByRole("grid", { name: "OpenShell Gateways" });
  await expect(
    gatewayGrid.getByRole("columnheader", { name: "Created" }),
  ).toBeVisible();
  await expect(gatewayGrid.getByText("Aug 10, 2026")).toBeVisible();
  await expect(gatewayGrid.locator(".pf-v6-c-label")).toHaveCount(0);
  await expect(
    page.getByRole("navigation", { name: "Primary navigation" }),
  ).toHaveCount(0);
  await page.getByRole("button", { name: "Refresh gateways" }).click();

  const results = await new AxeBuilder({ page }).analyze();
  expect(results.violations).toEqual([]);
});

test("keeps unknown gateway status readable in every theme", async ({
  page,
}) => {
  const unknownGateway = { ...gateway, status: "Future status" };
  await page.route("**/api/hypershell/v1/gateways**", async (route) => {
    const request = route.request();
    if (request.method() !== "GET") {
      await route.fallback();
      return;
    }

    const gatewayId = new URL(request.url()).pathname.split("/").at(-1);
    await route.fulfill({
      body: JSON.stringify(
        gatewayId === "gateways"
          ? {
              items: [unknownGateway],
              kind: "GatewayList",
              page: 1,
              size: 100,
              total: 1,
            }
          : unknownGateway,
      ),
      contentType: "application/json",
      status: 200,
    });
  });

  await page.goto("/");
  await expect(page.getByText("Future status", { exact: true })).toBeVisible();
  let results = await new AxeBuilder({ page }).analyze();
  expect(results.violations).toEqual([]);

  await page
    .getByRole("link", { name: "openshell-gateway-test", exact: true })
    .click();
  await expect(
    page.getByText("Future status", { exact: true }).first(),
  ).toBeVisible();
  results = await new AxeBuilder({ page }).analyze();
  expect(results.violations).toEqual([]);

  await page.getByRole("button", { name: "Switch to dark mode" }).click();
  await expect(page.locator("html")).toHaveClass(/pf-v6-theme-dark/u);
  results = await new AxeBuilder({ page }).analyze();
  expect(results.violations).toEqual([]);

  await page.goto("/");
  await expect(page.getByText("Future status", { exact: true })).toBeVisible();
  results = await new AxeBuilder({ page }).analyze();
  expect(results.violations).toEqual([]);
});

test("operates gateway rows and opens provisioning", async ({ page }) => {
  await page.goto("/");

  await page
    .getByRole("button", { name: "Actions for openshell-gateway-test" })
    .click();
  await expect(
    page.getByRole("menuitem", { name: "Open gateway console" }),
  ).toHaveCount(0);
  await expect(
    page.getByRole("menuitem", { name: "Copy CLI connection command" }),
  ).toBeVisible();

  await expect(
    page.getByRole("columnheader", { name: "Gateway name" }),
  ).toHaveAttribute("aria-sort", "ascending");
  await page.getByRole("button", { name: "Cluster" }).click();
  await expect(
    page.getByRole("columnheader", { name: "Cluster" }),
  ).toHaveAttribute("aria-sort", "ascending");
  await page
    .getByRole("textbox", {
      name: "Filter by name, cluster, status, or endpoint",
    })
    .fill("Cluster East");
  await expect(
    page.getByRole("link", { name: "openshell-gateway-test", exact: true }),
  ).toBeVisible();
  await expect(page).toHaveURL(/\?q=Cluster\+East&sort=cluster$/u);
  await page.goBack();
  await expect(page).toHaveURL(/\/$/u);
  await expect(
    page.getByRole("columnheader", { name: "Gateway name" }),
  ).toHaveAttribute("aria-sort", "ascending");

  await page.getByRole("link", { name: "Provision gateway" }).click();
  await expect(page).toHaveURL(/\/gateways\/new$/);
  await expect(page.getByRole("combobox", { name: "Cluster" })).toHaveValue(
    "Hub cluster (default)",
  );
  await expect(page.getByLabel("Namespace", { exact: true })).toHaveCount(0);
});

test("keeps connection methods on gateway details", async ({
  browserName,
  context,
  page,
}) => {
  if (browserName === "chromium") {
    await context.grantPermissions(["clipboard-read", "clipboard-write"]);
  }
  await page.goto("/");
  await page
    .getByRole("link", { name: "openshell-gateway-test", exact: true })
    .click();

  await expect(page).toHaveURL(/\/gateways\/openshell-gateway-test$/);
  await expect(page).toHaveTitle("HyperShell - Gateway details");
  await expect(
    page.getByRole("heading", { level: 1, name: "openshell-gateway-test" }),
  ).toBeFocused();
  await expect(
    page.getByRole("link", {
      name: "Open console for openshell-gateway-test in a new tab",
    }),
  ).toHaveCount(0);
  await expect(
    page.getByRole("button", { name: "Actions", exact: true }),
  ).toBeVisible();

  // Connection is the default tab and walks through login, provider, and sandbox.
  await expect(
    page.getByRole("tab", { name: "Connection", selected: true }),
  ).toBeVisible();
  await expect(
    page.getByRole("heading", { level: 2, name: "Log in to the gateway" }),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Copy the add-provider command" }),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Copy the create-sandbox command" }),
  ).toBeVisible();
  await page
    .getByRole("button", {
      name: "Copy connection command for openshell-gateway-test",
    })
    .click();
  await expect
    .poll(() => page.evaluate(() => navigator.clipboard.readText()))
    .toBe(
      `openshell gateway add \\
  --name openshell-gateway-test \\
  https://gateway.example.test:443`,
    );

  // Operational configuration and copyable values live under the Details tab.
  await page.getByRole("tab", { name: "Details" }).click();
  await expect(page.getByText("Cluster East", { exact: true })).toBeVisible();
  await expect(page.getByText("cluster-east", { exact: true })).toHaveCount(0);
  await expect(page.getByText("Not available")).toHaveCount(0);
  await page
    .getByRole("button", {
      name: "Copy gateway endpoint for openshell-gateway-test",
    })
    .click();
  await expect
    .poll(() => page.evaluate(() => navigator.clipboard.readText()))
    .toBe("https://gateway.example.test:443");

  await expect(
    page
      .getByRole("navigation", { name: "Breadcrumb" })
      .getByText("openshell-gateway-test"),
  ).toBeVisible();

  const results = await new AxeBuilder({ page }).analyze();
  expect(results.violations).toEqual([]);
});

test("renames a gateway from its row", async ({ page }) => {
  await page.goto("/");
  await page
    .getByRole("button", { name: "Actions for openshell-gateway-test" })
    .click();
  await page.getByRole("menuitem", { name: "Rename gateway" }).click();

  const dialog = page.getByRole("dialog", {
    name: "Rename openshell-gateway-test",
  });
  const nameInput = dialog.getByRole("textbox", { name: "Gateway name" });
  await expect(nameInput).toHaveValue("openshell-gateway-test");
  await nameInput.fill("team-gateway");
  const results = await new AxeBuilder({ page }).analyze();
  expect(results.violations).toEqual([]);
  await dialog.getByRole("button", { name: "Rename gateway" }).click();

  await expect(
    page.getByRole("link", { name: "team-gateway", exact: true }),
  ).toBeVisible();
  await expect(page.getByText("Gateway renamed to team-gateway")).toBeVisible();
});

test("renames a gateway from its details", async ({ page }) => {
  await page.goto("/gateways/openshell-gateway-test");
  await page.getByRole("button", { name: "Actions", exact: true }).click();
  await page.getByRole("menuitem", { name: "Rename gateway" }).click();
  const dialog = page.getByRole("dialog", {
    name: "Rename openshell-gateway-test",
  });
  await dialog
    .getByRole("textbox", { name: "Gateway name" })
    .fill("team-gateway");
  await dialog.getByRole("button", { name: "Rename gateway" }).click();

  await expect(
    page.getByRole("heading", { level: 1, name: "team-gateway" }),
  ).toBeVisible();
  await expect(
    page
      .getByRole("navigation", { name: "Breadcrumb" })
      .getByText("team-gateway"),
  ).toBeVisible();
  await expect(page.getByText("Gateway renamed to team-gateway")).toBeVisible();
});

test("deletes a gateway from its row after confirmation", async ({ page }) => {
  await page.goto("/");
  await page
    .getByRole("button", { name: "Actions for openshell-gateway-test" })
    .click();
  await page.getByRole("menuitem", { name: "Delete gateway" }).click();

  let dialog = page.getByRole("dialog", {
    name: "Delete openshell-gateway-test?",
  });
  await expect(dialog).toBeVisible();
  const results = await new AxeBuilder({ page }).analyze();
  expect(results.violations).toEqual([]);
  await dialog.getByRole("button", { name: "Cancel" }).click();
  await expect(
    page.getByRole("link", { name: "openshell-gateway-test", exact: true }),
  ).toBeVisible();

  await page
    .getByRole("button", { name: "Actions for openshell-gateway-test" })
    .click();
  await page.getByRole("menuitem", { name: "Delete gateway" }).click();
  dialog = page.getByRole("dialog", {
    name: "Delete openshell-gateway-test?",
  });
  await dialog.getByRole("button", { name: "Delete gateway" }).click();

  await expect(
    page.getByText("Gateway openshell-gateway-test deleted"),
  ).toBeVisible();
  await expect(
    page.getByRole("heading", { level: 2, name: "No gateways" }),
  ).toBeVisible();
});

test("deletes a gateway from its details and returns to the list", async ({
  page,
}) => {
  await page.goto("/gateways/openshell-gateway-test");
  await page.getByRole("button", { name: "Actions", exact: true }).click();
  await page.getByRole("menuitem", { name: "Delete gateway" }).click();
  const dialog = page.getByRole("dialog", {
    name: "Delete openshell-gateway-test?",
  });
  await dialog.getByRole("button", { name: "Delete gateway" }).click();

  await expect(page).toHaveURL(/\/$/u);
  await expect(
    page.getByText("Gateway openshell-gateway-test deleted"),
  ).toBeVisible();
  await expect(
    page.getByRole("heading", { level: 2, name: "No gateways" }),
  ).toBeVisible();
});

test("does not preserve removed administration routes", async ({ page }) => {
  for (const oldPath of [
    "/admin",
    "/admin/clusters",
    "/admin/gateways",
    "/admin/gateways/new",
  ]) {
    await page.goto(oldPath);
    await expect(page).toHaveURL(new RegExp(`${oldPath}$`));
    await expect(
      page.getByRole("heading", { level: 1, name: "Page not found" }),
    ).toBeVisible();
  }
});

test("provisions a gateway on an existing managed cluster", async ({
  page,
}) => {
  let requestBody: Record<string, unknown> | undefined;
  await page.route("**/api/hypershell/v1/gateways", async (route) => {
    if (route.request().method() !== "POST") {
      await route.continue();
      return;
    }

    requestBody = route.request().postDataJSON() as Record<string, unknown>;
    await route.fulfill({
      contentType: "application/json",
      status: 201,
      body: JSON.stringify({
        cluster_id: "cluster-east",
        created_at: null,
        database_id: "",
        external_dns: "",
        fleet_id: "",
        href: "/api/hypershell/v1/gateways/gateway-1",
        id: "gateway-1",
        kind: "Gateway",
        name: "team-gateway",
        namespace: "openshell",
        phase: "",
        release_id: "",
        service_type: "",
        status: "",
        tls_mode: "",
        updated_at: null,
      }),
    });
  });

  await page.goto("/");
  await page.getByRole("link", { name: "Provision gateway" }).click();
  await expect(
    page.getByRole("heading", { level: 1, name: "Provision gateway" }),
  ).toBeFocused();
  const clusterInput = page.getByRole("combobox", { name: "Cluster" });
  await expect(clusterInput).toHaveValue("Hub cluster (default)");
  await page.getByRole("button", { name: "Clear cluster search" }).click();
  await clusterInput.fill("East");
  await page.getByText("Cluster East", { exact: true }).click();
  await expect(clusterInput).toHaveValue("Cluster East");
  await expect(page.getByText("Provider: AWS; region: us-east-1")).toHaveCount(
    0,
  );
  await expect(page.getByLabel("Namespace", { exact: true })).toHaveCount(0);
  await expect(page.getByLabel("Gateway release")).toHaveCount(0);
  await expect(page.getByLabel("Managed database")).toHaveCount(0);
  await expect(page.getByText(/create|register/iu)).toHaveCount(0);
  await page.getByLabel("Gateway name").fill("team-gateway");
  await page.getByRole("button", { name: "Provision gateway" }).click();

  await expect(page).toHaveURL(/\/gateways\/gateway-1$/);
  await expect(
    page
      .getByRole("navigation", { name: "Breadcrumb" })
      .getByText("team-gateway"),
  ).toBeVisible();
  // Connection is the default tab; the login step shows a pending state until the
  // gateway reports an endpoint.
  await expect(
    page.getByRole("tab", { name: "Connection", selected: true }),
  ).toBeVisible();
  await expect(
    page.getByRole("status", {
      name: "Gateway login is unavailable until this gateway reports its endpoint and OIDC connection details.",
    }),
  ).toBeVisible();
  // Operational values remain under the Details tab; the endpoint is unavailable.
  await page.getByRole("tab", { name: "Details" }).click();
  await expect(
    page.getByText("Gateway endpoint", { exact: true }),
  ).toBeVisible();
  await expect(page.getByText("Not available")).toHaveCount(1);
  expect(requestBody).toEqual({
    cluster_id: "cluster-east",
    database_id: "",
    fleet_id: "",
    name: "team-gateway",
    release_id: "",
    route: '{"enabled":true}',
  });
});

test("shows the signed-in user and a full sign-out link", async ({ page }) => {
  await page.route("**/auth/session", async (route) => {
    await route.fulfill({
      body: JSON.stringify({
        authenticated: true,
        expires_at: 1_723_401_600,
        roles: ["hypershell-users"],
        user: { name: "Ada Lovelace", preferred_username: "ada" },
      }),
      contentType: "application/json",
      status: 200,
    });
  });

  await page.goto("/");
  await page.getByRole("button", { name: /Ada Lovelace/u }).click();

  // Sign-out is a real navigation to the BFF, which performs RP-initiated
  // Keycloak logout; it must not be a client-side route.
  const logout = page.getByRole("menuitem", { name: "Log out" });
  await expect(logout).toHaveAttribute("href", "/auth/logout");

  const results = await new AxeBuilder({ page }).analyze();
  expect(results.violations).toEqual([]);
});

test("reflows the gateway table without horizontal page overflow", async ({
  page,
}) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto("/");

  await expect(
    page.getByRole("heading", { level: 1, name: "OpenShell Gateways" }),
  ).toBeVisible();
  const hasOverflow = await page.evaluate(
    () =>
      document.documentElement.scrollWidth >
      document.documentElement.clientWidth,
  );
  expect(hasOverflow).toBe(false);

  const gatewayRow = page
    .getByRole("grid", { name: "OpenShell Gateways" })
    .getByRole("row")
    .filter({ hasText: "openshell-gateway-test" });
  const actionsButton = gatewayRow.getByRole("button", {
    name: "Actions for openshell-gateway-test",
  });
  const actionCell = actionsButton.locator("xpath=ancestor::td[1]");
  await expect(actionCell).toHaveClass(/pf-v6-c-table__action/u);

  const [rowBox, actionsBox] = await Promise.all([
    gatewayRow.boundingBox(),
    actionsButton.boundingBox(),
  ]);
  if (!rowBox || !actionsBox) {
    throw new Error("Expected the responsive gateway row action to be visible");
  }
  expect(actionsBox.y).toBeLessThan(rowBox.y + 48);
  expect(
    rowBox.x + rowBox.width - (actionsBox.x + actionsBox.width),
  ).toBeLessThan(48);

  const results = await new AxeBuilder({ page }).analyze();
  expect(results.violations).toEqual([]);
});
