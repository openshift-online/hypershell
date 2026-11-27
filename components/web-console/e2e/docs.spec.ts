import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Page } from "@playwright/test";

/*
 * Docs journeys (WEB-UDOC-13). Playwright runs against the Vite dev server, so
 * the BFF session resource and API are routed here; the production BFF public
 * docs exception is covered by the BFF tests.
 */

const runningGateway = {
  cluster_id: "cluster-east",
  created_at: "2026-11-24T10:00:00Z",
  external_dns: "gw-team.apps.example.test",
  gateway_version: "v0.0.120",
  href: "/api/hypershell/v1/gateways/gw-team",
  id: "gw-team",
  kind: "Gateway",
  name: "team-gateway",
  namespace: "openshell",
  oidc: JSON.stringify({
    audience: "openshell-api",
    client_id: "openshell-cli",
    issuer: "https://sso.example.test/realms/openshell",
  }),
  phase: "Running",
  status: "Ready",
};

async function routeSession(page: Page, authenticated: () => boolean) {
  await page.route("**/auth/session", async (route) => {
    await route.fulfill({
      body: JSON.stringify(
        authenticated()
          ? {
              authenticated: true,
              roles: ["hypershell-users"],
              user: { name: "Ada Lovelace", preferred_username: "ada" },
            }
          : { authenticated: false },
      ),
      contentType: "application/json",
      status: 200,
    });
  });
}

function recordApiRequests(page: Page): string[] {
  const requests: string[] = [];
  page.on("request", (request) => {
    const url = new URL(request.url());
    if (url.pathname.startsWith("/api/")) {
      requests.push(`${request.method()} ${url.pathname}`);
    }
  });
  return requests;
}

// Verifies: WEB-UDOC-02.01, WEB-UDOC-03, WEB-UDOC-09.01
test("a signed-out reader opens a refusal link without signing in", async ({
  page,
}) => {
  await routeSession(page, () => false);
  const apiRequests = recordApiRequests(page);

  await page.goto(
    "/docs/refusals/resource-limits?policy=sandbox-cpu-limit&gateway=team-a",
  );

  await expect(
    page.getByRole("heading", {
      level: 1,
      name: "Your sandbox asked for too much CPU or memory",
    }),
  ).toBeVisible();
  await expect(page.getByText("Rule: sandbox-cpu-limit")).toBeVisible();
  await expect(
    page.getByText("openshell sandbox create --name my-sandbox").first(),
  ).toBeVisible();
  await expect(
    page.getByRole("img", { name: "How a sandbox request is checked" }),
  ).toBeVisible();
  expect(apiRequests).toEqual([]);

  const results = await new AxeBuilder({ page }).analyze();
  expect(results.violations).toEqual([]);
});

// Verifies: WEB-UDOC-03.01, WEB-UDOC-03.02
test("signing in from a runbook cell returns the reader to the same page", async ({
  page,
}) => {
  let signedIn = false;
  await routeSession(page, () => signedIn);
  await page.route("**/api/hypershell/**", async (route) => {
    const url = new URL(route.request().url());
    if (url.pathname.endsWith("/placement-availability")) {
      await route.fulfill({
        body: JSON.stringify({
          placements: [
            { available: true, selector: ["network/public", "provider/ibm"] },
          ],
        }),
        contentType: "application/json",
        status: 200,
      });
      return;
    }
    if (url.pathname === "/api/hypershell/v1/gateways") {
      await route.fulfill({
        body: JSON.stringify({
          items: [],
          kind: "GatewayList",
          page: 1,
          size: 20,
          total: 0,
        }),
        contentType: "application/json",
        status: 200,
      });
      return;
    }
    await route.fulfill({
      body: "{}",
      contentType: "application/json",
      status: 403,
    });
  });
  // The BFF sign-in round trip: it stores the sanitized return_to and sends
  // the reader back there after the identity provider callback.
  await page.route("**/auth/login**", async (route) => {
    const returnTo = new URL(route.request().url()).searchParams.get(
      "return_to",
    );
    signedIn = true;
    await route.fulfill({
      headers: { location: returnTo ?? "/" },
      status: 302,
    });
  });

  await page.goto("/docs/runbooks/github-jira?name=team-gateway");
  const signIn = page.getByRole("link", {
    name: "Sign in to run this step here",
  });
  await expect(signIn).toHaveAttribute(
    "href",
    "/auth/login?return_to=%2Fdocs%2Frunbooks%2Fgithub-jira%3Fname%3Dteam-gateway",
  );

  await signIn.click();

  await expect(page).toHaveURL(
    /\/docs\/runbooks\/github-jira\?name=team-gateway$/u,
  );
  await expect(page.getByRole("button", { name: "Run" })).toBeVisible();
  await expect(page.getByLabel(/New gateway name/u)).toHaveValue(
    "team-gateway",
  );
});

// Verifies: WEB-UDOC-05.01, WEB-UDOC-05.02, WEB-UDOC-06
test("a signed-in reader runs Step 1 and gets the connection commands", async ({
  page,
}) => {
  await routeSession(page, () => true);
  let created = false;
  let createBody: unknown;
  await page.route("**/api/hypershell/**", async (route) => {
    const request = route.request();
    const url = new URL(request.url());
    if (url.pathname.endsWith("/placement-availability")) {
      await route.fulfill({
        body: JSON.stringify({
          placements: [
            {
              available: false,
              reason: "no-eligible-cluster",
              selector: ["provider/kind"],
            },
            { available: true, selector: ["network/public", "provider/ibm"] },
            { available: true, selector: ["network/public", "provider/aws"] },
          ],
        }),
        contentType: "application/json",
        status: 200,
      });
      return;
    }
    if (url.pathname === "/api/hypershell/v1/gateways") {
      if (request.method() === "POST") {
        created = true;
        createBody = request.postDataJSON();
        await route.fulfill({
          body: JSON.stringify({
            ...runningGateway,
            cluster_id: "",
            phase: "Pending",
          }),
          contentType: "application/json",
          status: 201,
        });
        return;
      }
      await route.fulfill({
        body: JSON.stringify({
          items: [],
          kind: "GatewayList",
          page: 1,
          size: 20,
          total: 0,
        }),
        contentType: "application/json",
        status: 200,
      });
      return;
    }
    if (url.pathname === "/api/hypershell/v1/gateways/gw-team") {
      await route.fulfill({
        body: JSON.stringify(runningGateway),
        contentType: "application/json",
        status: 200,
      });
      return;
    }
    await route.fulfill({
      body: "{}",
      contentType: "application/json",
      status: 403,
    });
  });

  await page.goto("/docs/runbooks/github-jira?name=team-gateway");
  const run = page.getByRole("button", { name: "Run" });
  await expect(run).toBeEnabled();
  // Prefill never submits (WEB-UDOC-05.01).
  expect(created).toBe(false);

  await run.click();

  // The detail poll runs every 5 seconds while the gateway is pending.
  await expect(page.getByText("Status: Running")).toBeVisible({
    timeout: 20_000,
  });
  expect(createBody).toEqual({
    name: "team-gateway",
    placement: {
      excludedSelector: ["runtime/kata"],
      selector: ["network/public", "provider/ibm"],
    },
  });
  await expect(page.getByRole("button", { name: "Re-check" })).toBeVisible();
  await expect(
    page.locator("code", { hasText: "openshell gateway add" }).first(),
  ).toContainText("--name team-gateway");

  const results = await new AxeBuilder({ page }).analyze();
  expect(results.violations).toEqual([]);
});
