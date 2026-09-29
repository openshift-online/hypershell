import { Nav, NavItem, NavList } from "@patternfly/react-core";
import { useIntl } from "react-intl";

import { useDashboardUi } from "../dashboard-ui-provider";
import { messages } from "../messages";

export type DashboardSecondaryNavActive = "operational" | "reliability";

export interface DashboardSecondaryNavProps {
  active: DashboardSecondaryNavActive;
}

const OPERATIONAL_HREF = "/dashboard";
const RELIABILITY_HREF = "/dashboard/reliability";

export function DashboardSecondaryNav({
  active,
}: Readonly<DashboardSecondaryNavProps>) {
  const intl = useIntl();
  const { navigation } = useDashboardUi();

  return (
    <Nav
      aria-label={intl.formatMessage(messages.reliabilityNavAriaLabel)}
      variant="horizontal-subnav"
    >
      <NavList>
        <NavItem
          isActive={active === "operational"}
          itemId="operational"
          onClick={(event) => {
            event.preventDefault();
            void navigation.navigate(OPERATIONAL_HREF);
          }}
          preventDefault
          to={OPERATIONAL_HREF}
        >
          {intl.formatMessage(messages.reliabilityNavOperational)}
        </NavItem>
        <NavItem
          isActive={active === "reliability"}
          itemId="reliability"
          onClick={(event) => {
            event.preventDefault();
            void navigation.navigate(RELIABILITY_HREF);
          }}
          preventDefault
          to={RELIABILITY_HREF}
        >
          {intl.formatMessage(messages.reliabilityNavReliability)}
        </NavItem>
      </NavList>
    </Nav>
  );
}
