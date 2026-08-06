import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { IntlProvider } from "react-intl";
import { MemoryRouter } from "react-router";

import { englishMessages } from "../../i18n/catalog";
import {
  AdminClustersPage,
  AdminGatewayPage,
  AdminGatewaysPage,
  AdminOverviewPage,
} from "./shell-pages";

function renderPage(Page: () => React.ReactNode) {
  return render(
    <IntlProvider locale="en" messages={englishMessages}>
      <MemoryRouter>
        <Page />
      </MemoryRouter>
    </IntlProvider>,
  );
}

describe("admin shell pages", () => {
  it("presents infrastructure entry points on the admin overview", () => {
    renderPage(AdminOverviewPage);

    expect(
      screen.getByRole("heading", { level: 1, name: "Administration" }),
    ).toBeTruthy();
    expect(
      screen.getByRole("link", { name: "View gateways" }).getAttribute("href"),
    ).toBe("/admin/gateways");
    expect(
      screen.getByRole("link", { name: "View clusters" }).getAttribute("href"),
    ).toBe("/admin/clusters");
    expect(screen.getAllByRole("heading", { level: 2 })).toHaveLength(2);
  });

  it("renders the gateway detail route purpose and empty state", () => {
    renderPage(AdminGatewayPage);

    expect(
      screen.getByRole("heading", {
        level: 1,
        name: "Gateway administration",
      }),
    ).toBeTruthy();
    expect(
      screen.getByRole("heading", {
        level: 2,
        name: "Gateway configuration is not available",
      }),
    ).toBeTruthy();
  });

  it("shows an empty state when there are no gateways to administer", () => {
    renderPage(() => <AdminGatewaysPage gateways={[]} />);

    expect(
      screen.getByRole("heading", {
        level: 2,
        name: "No gateways to administer",
      }),
    ).toBeTruthy();
  });

  it("shows the local cluster with a contextual provisioning action", () => {
    renderPage(AdminClustersPage);

    expect(
      screen.getByRole("heading", { level: 1, name: "Clusters" }),
    ).toBeTruthy();
    const clusterTable = screen.getByRole("grid", { name: "Clusters" });
    expect(within(clusterTable).getByText("Local cluster")).toBeTruthy();
    expect(screen.getByText("The cluster running HyperShell.")).toBeTruthy();
    expect(screen.queryByText("Registered")).toBeNull();
    expect(screen.queryByText(/only supported gateway placement/i)).toBeNull();
    expect(
      screen
        .getByRole("link", { name: "Provision gateway" })
        .getAttribute("href"),
    ).toBe("/admin/gateways/new?cluster=local");
    expect(screen.queryByText("No clusters registered")).toBeNull();
  });

  it("renders provisioning actions from cluster data", () => {
    renderPage(() => (
      <AdminClustersPage
        clusters={[
          {
            description: "A future remote placement.",
            id: "cluster-east",
            name: "East cluster",
          },
        ]}
      />
    ));

    expect(
      screen
        .getByRole("link", { name: "Provision gateway" })
        .getAttribute("href"),
    ).toBe("/admin/gateways/new?cluster=cluster-east");
  });

  it("filters clusters and clears a no-results state", async () => {
    const user = userEvent.setup();
    renderPage(AdminClustersPage);

    await user.type(
      screen.getByRole("textbox", { name: "Filter by name or description" }),
      "remote",
    );

    expect(
      screen.getByRole("heading", { level: 2, name: "No matching clusters" }),
    ).toBeTruthy();
    await user.click(screen.getByRole("button", { name: "Clear filters" }));
    expect(screen.getByText("Local cluster")).toBeTruthy();
  });

  it("paginates cluster collections", async () => {
    const user = userEvent.setup();
    renderPage(() => (
      <AdminClustersPage
        clusters={Array.from({ length: 21 }, (_value, index) => {
          const ordinal = String(index + 1);

          return {
            description: `Placement ${ordinal}`,
            id: `cluster-${ordinal}`,
            name: `Cluster ${ordinal}`,
          };
        })}
      />
    ));

    expect(screen.queryByText("Cluster 21")).toBeNull();
    await user.click(screen.getByRole("button", { name: "Go to next page" }));
    expect(screen.getByText("Cluster 21")).toBeTruthy();
  });

  it("sorts and filters the administrative gateway list", async () => {
    const user = userEvent.setup();
    renderPage(() => (
      <AdminGatewaysPage
        gateways={[
          {
            consoleUrl: "https://console.example/zulu",
            endpoint: "https://zulu.example:443",
            id: "zulu",
            name: "Zulu gateway",
            oidcAudience: "openshell-cli",
            oidcClientId: "openshell-cli",
            oidcIssuer: "https://issuer.example",
            status: "Ready",
          },
          {
            consoleUrl: "https://console.example/alpha",
            endpoint: "https://alpha.example:443",
            id: "alpha",
            name: "Alpha gateway",
            oidcAudience: "openshell-cli",
            oidcClientId: "openshell-cli",
            oidcIssuer: "https://issuer.example",
            status: "Pending",
          },
        ]}
      />
    ));

    let table = screen.getByRole("grid", { name: "Gateways" });
    expect(within(table).getAllByRole("row")[1]?.textContent).toContain(
      "Alpha gateway",
    );

    await user.click(
      within(table).getByRole("button", { name: "Gateway name" }),
    );
    table = screen.getByRole("grid", { name: "Gateways" });
    expect(within(table).getAllByRole("row")[1]?.textContent).toContain(
      "Zulu gateway",
    );

    await user.type(
      screen.getByRole("textbox", {
        name: "Filter by name, status, or endpoint",
      }),
      "Pending",
    );
    expect(screen.getByText("Alpha gateway")).toBeTruthy();
    expect(screen.queryByText("Zulu gateway")).toBeNull();
  });

  it("links gateway administration to provisioning", () => {
    renderPage(AdminGatewaysPage);

    expect(
      screen
        .getByRole("link", { name: "Provision gateway" })
        .getAttribute("href"),
    ).toBe("/admin/gateways/new");
  });
});
