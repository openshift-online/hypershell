import { render, screen } from "@testing-library/react";
import { IntlProvider } from "react-intl";
import { MemoryRouter, Route, Routes } from "react-router";
import { describe, expect, it, vi } from "vitest";

import type * as GatewayUi from "@openshift-online/hypershell-gateway-management-ui";
import { englishMessages } from "../i18n/catalog";

vi.mock("@openshift-online/hypershell-gateway-management-ui", async () => {
  const actual = await vi.importActual<typeof GatewayUi>(
    "@openshift-online/hypershell-gateway-management-ui",
  );
  return {
    ...actual,
    GatewaysPage: ({
      collectionState,
    }: {
      collectionState: { page: number; search: string };
    }) => {
      const renderedCollectionState = `${collectionState.search}:${String(
        collectionState.page,
      )}`;
      return <div data-testid="gateways-page">{renderedCollectionState}</div>;
    },
  };
});

import GatewaysRoute from "./gateways";

function renderRoute(path: string) {
  render(
    <IntlProvider locale="en" messages={englishMessages}>
      <MemoryRouter initialEntries={[path]}>
        <Routes>
          <Route element={<GatewaysRoute />} path="/gateways" />
        </Routes>
      </MemoryRouter>
    </IntlProvider>,
  );
}

describe("GatewaysRoute", () => {
  it("restores the gateway-list query state", () => {
    renderRoute("/gateways?q=team&page=2");

    expect(screen.getByTestId("gateways-page").textContent).toBe("team:2");
  });
});
