import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { IntlProvider } from "react-intl";
import { MemoryRouter } from "react-router";
import { beforeEach, expect, it, vi } from "vitest";

import { englishMessages } from "../../i18n/catalog";
import { UserMenu } from "./user-menu";

const { getSessionMock } = vi.hoisted(() => ({ getSessionMock: vi.fn() }));

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

beforeEach(() => {
  vi.clearAllMocks();
});

it("shows the user name and a full sign-out link", async () => {
  const user = userEvent.setup();
  getSessionMock.mockResolvedValue({
    authenticated: true,
    roles: ["hypershell-users"],
    user: { name: "Ada Lovelace", preferredUsername: "ada" },
  });

  renderMenu();

  const toggle = await screen.findByRole("button", { name: /Ada Lovelace/u });
  await user.click(toggle);

  const menu = screen.getByRole("menu");
  const logout = within(menu).getByRole("menuitem", { name: "Log out" });
  // Sign-out is a real navigation to the BFF endpoint, not a client route, so
  // the BFF can clear the session and perform RP-initiated Keycloak logout.
  expect(logout.getAttribute("href")).toBe("/auth/logout");
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
  expect(container.querySelector("button")).toBeNull();
});
