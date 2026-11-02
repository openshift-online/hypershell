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
  ["operational", "dashboardHref", messages.dashboardNavOverview, false],
  ["provisioning", "provisioningHref", messages.dashboardNavProvisioning, true],
  ["inventory", "inventoryHref", messages.dashboardNavInventory, true],
  ["reliability", "reliabilityHref", messages.reliabilityNavReliability, true],
  ["usage", "usageHref", messages.usageNavUsage, false],
  ["adoption", "adoptionHref", messages.dashboardNavAdoption, true],
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
          .map(([id, hrefName, label]) => (
            <NavItem
              isActive={active === id}
              itemId={id}
              key={id}
              onClick={(event) => {
                event.preventDefault();
                void navigation.navigate(navigation[hrefName]);
              }}
              preventDefault
              to={navigation[hrefName]}
            >
              {intl.formatMessage(label)}
            </NavItem>
          ))}
      </NavList>
    </Nav>
  );
}
