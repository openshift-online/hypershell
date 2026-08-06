import AxeBuilder from "@axe-core/playwright";
import { expect, test } from "@playwright/test";

const connectionCommand =
  "openshell gateway add --name openshell-gateway-test --oidc-issuer https://keycloak-openshell-keycloak.apps.rosa.hcmais01ue1.s9m2.p3.openshiftapps.com/realms/openshell --oidc-client-id openshell-cli --oidc-audience openshell-cli https://openshell-gw-openshell-gateway-test.apps.rosa.hcmais01ue1.s9m2.p3.openshiftapps.com:443";

const gateway = {
  cluster_id: "",
  created_at: null,
  database_id: "database-1",
  external_dns:
    "openshell-gw-openshell-gateway-test.apps.rosa.hcmais01ue1.s9m2.p3.openshiftapps.com",
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

test.beforeEach(async ({ page }) => {
  await page.route("**/api/hypershell/v1/gateways**", async (route) => {
    const request = route.request();
    if (request.method() !== "GET") {
      await route.continue();
      return;
    }

    const gatewayId = new URL(request.url()).pathname.split("/").at(-1);
    const body =
      gatewayId === "gateways"
        ? {
            items: [gateway],
            kind: "GatewayList",
            page: 1,
            size: 100,
            total: 1,
          }
        : gateway;
    await route.fulfill({
      body: JSON.stringify(body),
      contentType: "application/json",
      status: 200,
    });
  });
});

test("makes OpenShell connection methods the user-facing landing experience", async ({
  page,
}) => {
  await page.goto("/");

  await expect(
    page.getByRole("heading", { level: 1, name: "OpenShell gateways" }),
  ).toBeVisible();
  await expect(page).toHaveTitle("HyperShell — OpenShell gateways");
  await expect(
    page.getByRole("link", { name: "HyperShell", exact: true }),
  ).toBeVisible();
  await expect(
    page.getByRole("navigation", { name: "Primary navigation" }),
  ).toHaveCount(0);
  await expect(
    page.getByRole("link", { name: "Administration" }),
  ).toHaveAttribute("href", "/admin");
  await expect(
    page.getByRole("link", {
      name: "Open console for openshell-gateway-test in a new tab",
    }),
  ).toHaveAttribute(
    "href",
    "https://openshell-dashboard-openshell.apps.rosa.gkrumbac.9bpp.p3.openshiftapps.com",
  );
  await expect(
    page.getByRole("button", {
      name: "Copy connection command for openshell-gateway-test",
    }),
  ).toBeVisible();
  await page.getByRole("button", { name: "Refresh gateways" }).click();
  await expect(
    page.getByRole("link", { name: "openshell-gateway-test", exact: true }),
  ).toBeVisible();

  const results = await new AxeBuilder({ page }).analyze();
  expect(results.violations).toEqual([]);
});

test("copies the complete CLI connection command", async ({
  context,
  page,
}) => {
  await context.grantPermissions(["clipboard-read", "clipboard-write"]);
  await page.goto("/");

  await page
    .getByRole("button", {
      name: "Copy connection command for openshell-gateway-test",
    })
    .click();

  await expect
    .poll(() => page.evaluate(() => navigator.clipboard.readText()))
    .toBe(connectionCommand);
});

test("cross-navigates between the gateway directory and administration", async ({
  page,
}) => {
  await page.goto("/");

  await page.getByRole("link", { name: "Administration" }).click();
  await expect(page).toHaveURL(/\/admin$/);

  await page.getByRole("link", { name: "Gateway directory" }).click();
  await expect(page).toHaveURL(/\/$/);
});

test("keeps connection methods available on gateway details", async ({
  page,
}) => {
  await page.goto("/");

  await page
    .getByRole("link", { name: "openshell-gateway-test", exact: true })
    .click();

  await expect(page).toHaveURL(/\/gateways\/openshell-gateway-test$/);
  await expect(page).toHaveTitle("HyperShell — Gateway details");
  await expect(
    page.getByRole("heading", { level: 1, name: "openshell-gateway-test" }),
  ).toBeFocused();
  await expect(page.getByRole("textbox")).toHaveValue(connectionCommand);
  const breadcrumb = page.getByRole("navigation", { name: "Breadcrumb" });
  await expect(
    breadcrumb.getByRole("link", { name: "Gateways" }),
  ).toBeVisible();

  const results = await new AxeBuilder({ page }).analyze();
  expect(results.violations).toEqual([]);
});

test("makes the gateway collection the administration landing page", async ({
  page,
}) => {
  await page.goto("/admin");

  await expect(page).toHaveTitle("HyperShell Administration — Gateways");
  await expect(
    page.getByRole("link", { name: "HyperShell Administration" }),
  ).toBeVisible();
  await expect(
    page.getByRole("link", { name: "Gateway directory" }),
  ).toHaveAttribute("href", "/");
  await expect(
    page.getByRole("navigation", { name: "Primary navigation" }),
  ).toHaveCount(0);
  await expect(
    page.getByRole("heading", { level: 1, name: "Gateways" }),
  ).toBeVisible();
  await expect(
    page.getByRole("link", { name: "Provision gateway" }),
  ).toBeVisible();
  await expect(
    page.getByRole("grid", { name: "Gateways" }).getByText("Local cluster"),
  ).toBeVisible();
  await expect(page.getByText("Connect with the CLI")).toHaveCount(0);

  const results = await new AxeBuilder({ page }).analyze();
  expect(results.violations).toEqual([]);
});

test("searches, sorts, refreshes, and provisions from administration", async ({
  page,
}) => {
  await page.goto("/admin");

  await expect(
    page.getByRole("grid", { name: "Gateways" }).getByText("Local cluster"),
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
    .fill("Local cluster");
  await expect(
    page.getByRole("link", { name: "openshell-gateway-test", exact: true }),
  ).toBeVisible();
  await page.getByRole("button", { name: "Refresh gateways" }).click();
  await expect(page).toHaveURL(/\/admin$/);

  const results = await new AxeBuilder({ page }).analyze();
  expect(results.violations).toEqual([]);

  await page.getByRole("link", { name: "Provision gateway" }).click();
  await expect(page).toHaveURL(/\/admin\/gateways\/new$/);
  await expect(page.getByLabel("Cluster", { exact: true })).toHaveCount(0);
});

test("does not preserve removed administration routes as redirects", async ({
  page,
}) => {
  for (const oldPath of ["/admin/clusters", "/admin/gateways"]) {
    await page.goto(oldPath);
    await expect(page).toHaveURL(new RegExp(`${oldPath}$`));
    await expect(
      page.getByRole("heading", { level: 1, name: "Page not found" }),
    ).toBeVisible();
  }
});

test("provisions a gateway without exposing placement fields", async ({
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
        cluster_id: "",
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

  await page.goto("/admin");
  await expect(
    page.getByRole("button", { name: "Refresh gateways" }),
  ).toBeVisible();
  await expect(
    page.getByRole("textbox", {
      name: "Filter by name, cluster, status, or endpoint",
    }),
  ).toBeVisible();
  await expect(
    page.getByRole("columnheader", { name: "Gateway name" }),
  ).toHaveAttribute("aria-sort", "ascending");
  await expect(page.getByRole("link", { name: "Provision gateway" })).toHaveCSS(
    "color",
    "rgb(255, 255, 255)",
  );
  await page.getByRole("link", { name: "Provision gateway" }).click();

  await expect(
    page.getByRole("heading", { level: 1, name: "Provision gateway" }),
  ).toBeFocused();
  await expect(page.getByLabel("Cluster", { exact: true })).toHaveCount(0);
  await expect(page.getByLabel("Gateway release")).toHaveCount(0);
  await expect(page.getByLabel("Managed database")).toHaveCount(0);
  await expect(
    page.getByRole("button", { name: "Provision gateway" }),
  ).not.toHaveClass(/pf-m-progress/);
  await page.getByLabel("Gateway name").fill("team-gateway");
  await page.getByRole("button", { name: "Provision gateway" }).click();

  await expect(page).toHaveURL(/\/admin\/gateways\/gateway-1$/);
  expect(requestBody).toEqual({
    cluster_id: "",
    database_id: "",
    fleet_id: "",
    name: "team-gateway",
    namespace: "openshell",
    release_id: "",
  });
});

test("reflows the gateway directory without horizontal page overflow", async ({
  page,
}) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto("/");

  await expect(
    page.getByRole("heading", { level: 1, name: "OpenShell gateways" }),
  ).toBeVisible();
  const hasOverflow = await page.evaluate(
    () =>
      document.documentElement.scrollWidth >
      document.documentElement.clientWidth,
  );
  expect(hasOverflow).toBe(false);

  const results = await new AxeBuilder({ page }).analyze();
  expect(results.violations).toEqual([]);
});
