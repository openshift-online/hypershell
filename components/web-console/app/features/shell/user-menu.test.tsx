import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { IntlProvider } from "react-intl";
import { MemoryRouter } from "react-router";
import { beforeEach, expect, it, vi } from "vitest";

import { englishMessages } from "../../i18n/catalog";
import { UserMenu } from "./user-menu";

const { getSessionMock, readVersionMock } = vi.hoisted(() => ({
  getSessionMock: vi.fn(),
  readVersionMock: vi.fn(),
}));

vi.mock("../../composition/api-version-composition", () => ({
  apiVersionReader: { readVersion: readVersionMock },
}));

vi.mock("../../composition/session-composition", () => ({
  sessionGateway: { getSession: getSessionMock },
}));

function renderMenu() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <IntlProvider locale="en" messages={englishMessages}>
      <QueryClientProvider client={queryClient}>
        <MemoryRouter>
          <UserMenu />
        </MemoryRouter>
      </QueryClientProvider>
    </IntlProvider>,
  );
}

function setBuildVersion(version: string) {
  const meta = document.createElement("meta");
  meta.setAttribute("name", "hypershell-runtime-config");
  meta.setAttribute(
    "content",
    JSON.stringify({ build: { version }, tracing: { sampleRatio: 0 } }),
  );
  document.head.append(meta);
}

beforeEach(() => {
  vi.clearAllMocks();
  readVersionMock.mockResolvedValue("v1.6.0-7654321");
  document.querySelector('meta[name="hypershell-runtime-config"]')?.remove();
});

it("shows the user name, image versions, and a full sign-out link", async () => {
  const user = userEvent.setup();
  getSessionMock.mockResolvedValue({
    authenticated: true,
    roles: ["hypershell-users"],
    user: { name: "Ada Lovelace", preferredUsername: "ada" },
  });
  setBuildVersion("v1.6.0-1234567");

  renderMenu();

  const toggle = await screen.findByRole("button", { name: /Ada Lovelace/u });
  await user.click(toggle);

  const menu = screen.getByRole("menu");
  const consoleVersion = within(menu).getByRole("menuitem", {
    name: "Console version v1.6.0-1234567",
  });
  const apiVersion = await within(menu).findByRole("menuitem", {
    name: "API version v1.6.0-7654321",
  });
  const logout = within(menu).getByRole("menuitem", { name: "Log out" });
  expect((consoleVersion as HTMLButtonElement).disabled).toBe(true);
  expect(consoleVersion.getAttribute("href")).toBeNull();
  expect((apiVersion as HTMLButtonElement).disabled).toBe(true);
  expect(apiVersion.getAttribute("href")).toBeNull();
  // Sign-out is a real navigation to the BFF endpoint, not a client route, so
  // the BFF can clear the session and perform RP-initiated Keycloak logout.
  expect(logout.getAttribute("href")).toBe("/auth/logout");
});

it("shows unknown versions and keeps logout available", async () => {
  const user = userEvent.setup();
  getSessionMock.mockResolvedValue({
    authenticated: true,
    roles: ["hypershell-users"],
    user: { name: "Ada Lovelace" },
  });
  readVersionMock.mockRejectedValue(new Error("API unavailable"));
  setBuildVersion("latest");

  renderMenu();

  await user.click(
    await screen.findByRole("button", { name: /Ada Lovelace/u }),
  );
  const menu = screen.getByRole("menu");
  expect(
    within(menu).getByRole("menuitem", {
      name: "Console version unknown",
    }),
  ).toBeTruthy();
  expect(
    within(menu).getByRole("menuitem", {
      name: "API version unknown",
    }),
  ).toBeTruthy();
  expect(within(menu).getByRole("menuitem", { name: "Log out" })).toBeTruthy();
});

it("keeps the build version that it reads when it mounts", async () => {
  const user = userEvent.setup();
  getSessionMock.mockResolvedValue({
    authenticated: true,
    roles: ["hypershell-users"],
    user: { name: "Ada Lovelace" },
  });
  setBuildVersion("v1.6.0-1234567");

  renderMenu();

  const toggle = await screen.findByRole("button", { name: /Ada Lovelace/u });
  document
    .querySelector('meta[name="hypershell-runtime-config"]')
    ?.setAttribute(
      "content",
      JSON.stringify({
        build: { version: "v1.6.0-7654321" },
        tracing: { sampleRatio: 0 },
      }),
    );
  await user.click(toggle);

  expect(
    within(screen.getByRole("menu")).getByRole("menuitem", {
      name: "Console version v1.6.0-1234567",
    }),
  ).toBeTruthy();
});

it("falls back to the preferred username, then email, then Account", async () => {
  getSessionMock.mockResolvedValue({
    authenticated: true,
    roles: [],
    user: { email: "person@example.test" },
  });

  renderMenu();

  expect(
    await screen.findByRole("button", { name: /person@example.test/u }),
  ).toBeTruthy();
});

it("renders nothing when unauthenticated", async () => {
  getSessionMock.mockResolvedValue({ authenticated: false, roles: [] });

  const { container } = renderMenu();

  // Allow the query to settle, then confirm no toggle was rendered.
  await vi.waitFor(() => {
    expect(getSessionMock).toHaveBeenCalled();
  });
  expect(readVersionMock).not.toHaveBeenCalled();
  expect(container.querySelector("button")).toBeNull();
});
