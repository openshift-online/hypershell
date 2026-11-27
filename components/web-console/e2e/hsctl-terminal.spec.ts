import { expect, test, type Page, type Request } from "@playwright/test";

/*
 * Browser journeys for the hsctl terminal drawer (WEB-TERM-14) against the
 * Vite dev server with mocked session and API responses. The production-BFF
 * command journey, the recording upstream, and the CSP split are covered by
 * the BFF tests.
 */

const gateway = {
  cluster_id: "cluster-east",
  created_at: "2026-08-10T14:30:00Z",
  external_dns: "gateway.example.test",
  href: "/api/hypershell/v1/gateways/openshell-gateway-test",
  id: "openshell-gateway-test",
  kind: "Gateway",
  name: "openshell-gateway-test",
  namespace: "openshell",
  phase: "Running",
  service_type: "",
  status: "Ready",
  tls_mode: "",
  updated_at: null,
};

const runtimeAssetPattern =
  /hsctl-worker|wasm_exec|hsctl(-[A-Za-z0-9_-]+)?\.wasm|\.hsctl-runtime/u;

async function mockConsole(
  page: Page,
  session: "signed-in" | "signed-out",
): Promise<void> {
  await page.route("**/auth/session", async (route) => {
    await route.fulfill({
      body: JSON.stringify(
        session === "signed-in"
          ? {
              authenticated: true,
              expires_at: 4_102_444_800,
              roles: ["hypershell-users"],
              user: { name: "Ada Lovelace", sub: "ada" },
            }
          : { authenticated: false },
      ),
      contentType: "application/json",
      status: 200,
    });
  });
  await page.route("**/api/hypershell/v1/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    const body = path.endsWith("/gateways")
      ? { items: [gateway], kind: "GatewayList", page: 1, size: 100, total: 1 }
      : path.includes("/gateways/")
        ? gateway
        : { items: [], kind: "List", page: 1, size: 0, total: 0 };
    await route.fulfill({
      body: JSON.stringify(body),
      contentType: "application/json",
      status: 200,
    });
  });
}

function recordRuntimeRequests(page: Page): Request[] {
  const requests: Request[] = [];
  page.context().on("request", (request) => {
    if (runtimeAssetPattern.test(new URL(request.url()).pathname)) {
      requests.push(request);
    }
  });
  return requests;
}

// Verifies: WEB-TERM-09.01
test("requests no runtime asset before the drawer opens, then each at most once", async ({
  page,
}) => {
  await mockConsole(page, "signed-in");
  const runtimeRequests = recordRuntimeRequests(page);

  await page.goto("/gateways");
  await expect(
    page.getByRole("link", { name: "openshell-gateway-test", exact: true }),
  ).toBeVisible();
  await page.waitForLoadState("networkidle");
  expect(runtimeRequests).toHaveLength(0);

  await page.getByRole("button", { name: "Open hsctl terminal" }).click();
  const region = page.getByRole("region", { name: "hsctl terminal" });
  await expect(region).toBeVisible();
  // A dev server without runtime files shows the localized load error; with
  // them, the input becomes editable once the runtime has started.
  await expect(
    region
      .getByText("hsctl could not be loaded")
      .or(region.getByRole("textbox", { name: "hsctl command", exact: true })),
  ).toBeVisible();
  await page.waitForLoadState("networkidle");
  const urls = runtimeRequests.map((request) => request.url());
  expect(new Set(urls).size).toBe(urls.length);
});

// Verifies: WEB-TERM-01.01
test("toggles with Ctrl+` and returns focus to the gateway list link on Esc", async ({
  page,
}) => {
  await mockConsole(page, "signed-in");
  await page.goto("/gateways");
  const link = page.getByRole("link", {
    name: "openshell-gateway-test",
    exact: true,
  });
  await link.focus();

  await page.keyboard.press("Control+Backquote");

  const input = page.getByRole("textbox", { name: "hsctl command" });
  await expect(input).toBeFocused();
  await expect(
    page.getByRole("button", { name: "Close hsctl terminal" }),
  ).toBeVisible();

  await page.keyboard.press("Escape");

  await expect(
    page.getByRole("region", { name: "hsctl terminal" }),
  ).toHaveCount(0);
  await expect(link).toBeFocused();
});

// Verifies: WEB-TERM-03.01
test("clamps keyboard resizing at 180 px", async ({ page }) => {
  await mockConsole(page, "signed-in");
  await page.addInitScript(() => {
    localStorage.setItem("hypershell.terminal.dock", "bottom");
    localStorage.setItem(
      "hypershell.terminal.sizes",
      JSON.stringify({ bottom: 190, right: 480 }),
    );
  });
  await page.goto("/gateways");
  await page.getByRole("button", { name: "Open hsctl terminal" }).click();

  const handle = page.getByRole("separator", { name: "Resize hsctl terminal" });
  await expect(handle).toHaveAttribute("aria-valuenow", "190");
  await handle.focus();
  await page.keyboard.press("ArrowDown");

  await expect(handle).toHaveAttribute("aria-valuenow", "180");
  const panel = page.getByRole("region", { name: "hsctl terminal" });
  const box = await panel.boundingBox();
  expect(Math.round(box?.height ?? 0)).toBe(180);
});

// Verifies: WEB-TERM-02.01
test("docks right beside the page and reopens docked right after a reload", async ({
  page,
}) => {
  await page.setViewportSize({ height: 900, width: 1440 });
  await mockConsole(page, "signed-in");
  await page.goto("/gateways");
  await page.getByRole("button", { name: "Open hsctl terminal" }).click();
  await page.getByRole("button", { name: "Dock right" }).click();

  const panel = page.getByRole("region", { name: "hsctl terminal" });
  const heading = page.getByRole("heading", { level: 1 });
  const panelBox = await panel.boundingBox();
  const headingBox = await heading.boundingBox();
  expect(panelBox).not.toBeNull();
  expect(headingBox).not.toBeNull();
  if (panelBox && headingBox) {
    expect(headingBox.x + headingBox.width).toBeLessThanOrEqual(panelBox.x);
  }

  await page.reload();
  await page.getByRole("button", { name: "Open hsctl terminal" }).click();
  await expect(page.getByRole("button", { name: "Dock bottom" })).toBeVisible();
});

// Verifies: WEB-TERM-01.02
test("shows a signed-out reader only the sign-in link and loads no runtime", async ({
  page,
}) => {
  await mockConsole(page, "signed-out");
  const runtimeRequests = recordRuntimeRequests(page);
  await page.goto("/docs/runbooks/github-jira?name=team-gateway");

  await page.getByRole("button", { name: "Open hsctl terminal" }).click();

  const region = page.getByRole("region", { name: "hsctl terminal" });
  const signIn = region.getByRole("link", {
    name: "Sign in to use the hsctl terminal",
  });
  await expect(signIn).toBeVisible();
  await expect(signIn).toHaveAttribute(
    "href",
    "/auth/login?return_to=%2Fdocs%2Frunbooks%2Fgithub-jira%3Fname%3Dteam-gateway",
  );
  await expect(region.getByRole("textbox")).toHaveCount(0);
  await page.waitForLoadState("networkidle");
  expect(runtimeRequests).toHaveLength(0);
});
