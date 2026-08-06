import AxeBuilder from "@axe-core/playwright";
import { expect, test } from "@playwright/test";

const connectionCommand =
  "openshell gateway add --name openshell-gateway-test --oidc-issuer https://keycloak-openshell-keycloak.apps.rosa.hcmais01ue1.s9m2.p3.openshiftapps.com/realms/openshell --oidc-client-id openshell-cli --oidc-audience openshell-cli https://openshell-gw-openshell-gateway-test.apps.rosa.hcmais01ue1.s9m2.p3.openshiftapps.com:443";

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

test("keeps infrastructure and provisioning concepts in the admin shell", async ({
  page,
}) => {
  await page.goto("/admin");

  await expect(page).toHaveTitle("HyperShell Administration — Administration");
  await expect(
    page.getByRole("link", { name: "HyperShell Administration" }),
  ).toBeVisible();
  await expect(
    page.getByRole("link", { name: "Gateway directory" }),
  ).toHaveAttribute("href", "/");
  const navigation = page.getByRole("navigation", {
    name: "Primary navigation",
  });
  await expect(
    navigation.getByRole("link", { name: "Clusters" }),
  ).toBeVisible();
  await expect(
    navigation.getByRole("link", { name: "Gateways" }),
  ).toBeVisible();
  await expect(page.getByText("Connect with the CLI")).toHaveCount(0);

  const results = await new AxeBuilder({ page }).analyze();
  expect(results.violations).toEqual([]);
});

test("navigates within administration and focuses the new page", async ({
  page,
}) => {
  await page.goto("/admin");

  await page.getByRole("link", { name: "Clusters", exact: true }).click();

  await expect(page).toHaveURL(/\/admin\/clusters$/);
  await expect(
    page.getByRole("heading", { level: 1, name: "Clusters" }),
  ).toBeFocused();
  await expect(
    page.getByRole("grid", { name: "Clusters" }).getByText("Local cluster"),
  ).toBeVisible();
  await expect(
    page.getByRole("columnheader", { name: "Name" }),
  ).toHaveAttribute("aria-sort", "ascending");
  await page.getByRole("button", { name: "Description" }).click();
  await expect(
    page.getByRole("columnheader", { name: "Description" }),
  ).toHaveAttribute("aria-sort", "ascending");
  await page
    .getByRole("textbox", { name: "Filter by name or description" })
    .fill("not-a-cluster");
  await expect(
    page.getByRole("heading", { level: 2, name: "No matching clusters" }),
  ).toBeVisible();
  await page.getByRole("button", { name: "Clear filters" }).click();
  await expect(page.getByText("Registered", { exact: true })).toHaveCount(0);

  const results = await new AxeBuilder({ page }).analyze();
  expect(results.violations).toEqual([]);

  await page.getByRole("link", { name: "Provision gateway" }).click();
  await expect(page).toHaveURL(/\/admin\/gateways\/new\?cluster=local$/);
  await expect(page.getByLabel("Cluster", { exact: true })).toHaveValue(
    "Local cluster",
  );
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
        database_id: "database-1",
        external_dns: "",
        fleet_id: "",
        href: "/api/hypershell/v1/gateways/gateway-1",
        id: "gateway-1",
        kind: "Gateway",
        name: "team-gateway",
        namespace: "openshell",
        phase: "",
        release_id: "release-1",
        service_type: "",
        status: "",
        tls_mode: "",
        updated_at: null,
      }),
    });
  });

  await page.goto("/admin/gateways");
  await expect(
    page.getByRole("textbox", {
      name: "Filter by name, status, or endpoint",
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
  await expect(page.getByLabel("Cluster", { exact: true })).toHaveValue(
    "Local cluster",
  );
  await expect(
    page.getByRole("button", { name: "Provision gateway" }),
  ).not.toHaveClass(/pf-m-progress/);
  await page.getByLabel("Gateway name").fill("team-gateway");
  await page.getByLabel("Gateway release ID").fill("release-1");
  await page.getByLabel("Managed database ID").fill("database-1");
  await page.getByRole("button", { name: "Provision gateway" }).click();

  await expect(page).toHaveURL(/\/admin\/gateways\/gateway-1$/);
  expect(requestBody).toEqual({
    database_id: "database-1",
    name: "team-gateway",
    namespace: "openshell",
    release_id: "release-1",
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
