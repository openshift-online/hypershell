import { Nav, NavItem, NavList } from "@patternfly/react-core";
import { useIntl } from "react-intl";

import { useDashboardUi } from "../dashboard-ui-provider";
import { messages } from "../messages";

export type DashboardSecondaryNavActive =
  | "operational"
  | "provisioning"
  | "inventory"
  | "reliability"
  | "usage"
  | "adoption";

export interface DashboardSecondaryNavProps {
  active: DashboardSecondaryNavActive;
}

const items = [
  ["operational", "/dashboard", messages.dashboardNavOverview, false],
  [
    "provisioning",
    "/dashboard/provisioning",
    messages.dashboardNavProvisioning,
    true,
  ],
  ["inventory", "/dashboard/inventory", messages.dashboardNavInventory, true],
  [
    "reliability",
    "/dashboard/reliability",
    messages.reliabilityNavReliability,
    true,
  ],
  ["usage", "/dashboard/usage", messages.usageNavUsage, false],
  ["adoption", "/dashboard/adoption", messages.dashboardNavAdoption, true],
] as const;

export function DashboardSecondaryNav({
  active,
}: Readonly<DashboardSecondaryNavProps>) {
  const intl = useIntl();
  const { isAdmin, navigation } = useDashboardUi();

  return (
    <Nav
      aria-label={intl.formatMessage(messages.reliabilityNavAriaLabel)}
      variant="horizontal-subnav"
    >
      <NavList>
        {items
          .filter(([, , , adminOnly]) => !adminOnly || isAdmin)
          .map(([id, href, label]) => (
            <NavItem
              isActive={active === id}
              itemId={id}
              key={id}
              onClick={(event) => {
                event.preventDefault();
                void navigation.navigate(href);
              }}
              preventDefault
              to={href}
            >
              {intl.formatMessage(label)}
            </NavItem>
          ))}
      </NavList>
    </Nav>
  );
}
