import AxeBuilder from "@axe-core/playwright";
import { expect, test } from "@playwright/test";

test("renders the localized Hello world route without detectable accessibility violations", async ({
  page,
}) => {
  await page.goto("/");

  await expect(
    page.getByRole("heading", { level: 1, name: "Hello world" }),
  ).toBeVisible();
  const results = await new AxeBuilder({ page }).analyze();
  expect(results.violations).toEqual([]);
});

test("supports a direct nested fleet route", async ({ page }) => {
  const response = await page.goto("/fleets/example/gateways/example-gateway");

  expect(response?.status()).toBe(200);
  await expect(
    page.getByRole("heading", { level: 1, name: "Hello world" }),
  ).toBeVisible();
});
