import { Nav, NavItem, NavList } from "@patternfly/react-core";
import { useIntl } from "react-intl";

import { useDashboardUi } from "../dashboard-ui-provider";
import { costMessages } from "../cost-messages";
import { messages } from "../messages";

export type DashboardSecondaryNavActive =
  | "operational"
  | "provisioning"
  | "inventory"
  | "reliability"
  | "usage"
  | "adoption"
  | "cost";

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
  ["cost", "costHref", costMessages.costNavCost, false],
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
          .flatMap(([id, hrefName, label]) => {
            const href = navigation[hrefName];
            if (href === undefined) {
              return [];
            }
            return [
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
              </NavItem>,
            ];
          })}
      </NavList>
    </Nav>
  );
}
