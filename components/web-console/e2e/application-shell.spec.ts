import AxeBuilder from "@axe-core/playwright";
import { expect, test } from "@playwright/test";

test("renders the application shell without detectable accessibility violations", async ({
  page,
}) => {
  await page.goto("/");

  await expect(
    page.getByRole("heading", { level: 1, name: "Overview" }),
  ).toBeVisible();
  await expect(page).toHaveTitle("HyperShell — Overview");
  await expect(
    page.getByRole("navigation", { name: "Primary navigation" }),
  ).toBeVisible();
  await expect(page.getByText("Development preview")).toBeVisible();
  await expect(page.getByRole("button", { name: /Select sector/ })).toHaveCount(
    0,
  );

  const results = await new AxeBuilder({ page }).analyze();
  expect(results.violations).toEqual([]);
});

test("navigates between global destinations and focuses the page heading", async ({
  page,
}) => {
  await page.goto("/");

  await page.getByRole("link", { name: "Sectors", exact: true }).click();

  await expect(page).toHaveURL(/\/fleets$/);
  await expect(
    page.getByRole("heading", { level: 1, name: "Sectors" }),
  ).toBeFocused();
  await expect(page.getByRole("button", { name: /Select sector/ })).toHaveCount(
    0,
  );
});

test("supports a direct sector deep link on the current URL contract", async ({
  page,
}) => {
  const response = await page.goto("/fleets/example/gateways/example-gateway");

  expect(response?.status()).toBe(200);
  await expect(page).toHaveTitle("HyperShell — Gateway details");
  await expect(
    page.getByRole("heading", { level: 1, name: "Gateway details" }),
  ).toBeVisible();
  await expect(
    page.getByRole("button", {
      name: "Select sector, currently example",
    }),
  ).toBeVisible();
  await expect(
    page.getByRole("navigation", { name: "Breadcrumb" }),
  ).toContainText("Sector example");
  await expect(
    page.getByRole("navigation", { name: "Breadcrumb" }),
  ).toContainText("Gateway example-gateway");

  const results = await new AxeBuilder({ page }).analyze();
  expect(results.violations).toEqual([]);
});

test("switches sector context without carrying a gateway identifier", async ({
  page,
}) => {
  await page.route("**/api/hypershell/v1/fleets?*", async (route) => {
    await route.fulfill({
      contentType: "application/json",
      json: {
        items: [
          { id: "sector-a", name: "Alpha sector" },
          { id: "sector-b", name: "Beta sector" },
        ],
        kind: "FleetList",
        page: 1,
        size: 100,
        total: 2,
      },
    });
  });
  await page.goto("/fleets/sector-a/gateways/gateway-a");

  await page
    .getByRole("button", {
      name: "Select sector, currently Alpha sector",
    })
    .click();
  await page.getByRole("option", { name: "Beta sector" }).click();

  await expect(page).toHaveURL(/\/fleets\/sector-b\/gateways$/);
  await expect(
    page.getByRole("heading", { level: 1, name: "Gateways" }),
  ).toBeFocused();
});

test("opens and uses primary navigation at a narrow viewport", async ({
  page,
}) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto("/");

  const navigation = page.getByRole("navigation", {
    name: "Primary navigation",
  });
  await expect(navigation).toBeHidden();
  await page.getByRole("button", { name: "Toggle primary navigation" }).click();
  await expect(navigation).toBeVisible();
  await navigation.getByRole("link", { name: "Sectors", exact: true }).click();

  await expect(page).toHaveURL(/\/fleets$/);
});
