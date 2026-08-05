import {
  Breadcrumb,
  BreadcrumbItem,
  Label,
  Masthead,
  MastheadBrand,
  MastheadContent,
  MastheadLogo,
  MastheadMain,
  MastheadToggle,
  Nav,
  NavGroup,
  NavItem,
  NavList,
  Page,
  PageSidebar,
  PageSidebarBody,
  PageToggleButton,
  SkipToContent,
  Toolbar,
  ToolbarContent,
  ToolbarGroup,
  ToolbarItem,
} from "@patternfly/react-core";
import { useEffect, useRef } from "react";
import { FormattedMessage, useIntl } from "react-intl";
import { Link, Outlet, useLocation } from "react-router";

import { messages } from "../../i18n/messages";
import styles from "./application-shell.module.css";
import { SectorContextBar } from "./sector-selector";

interface NavigationItemProps {
  isActive: boolean;
  label: React.ReactNode;
  to: string;
}

function NavigationItem({ isActive, label, to }: NavigationItemProps) {
  return (
    <NavItem isActive={isActive} itemId={to} to={to}>
      <Link to={to}>{label}</Link>
    </NavItem>
  );
}

export function ApplicationShell() {
  const intl = useIntl();
  const { pathname } = useLocation();
  const previousPath = useRef(pathname);
  const segments = pathname.split("/").filter(Boolean);
  // Fleet URLs remain the transport contract until the Sector API and SDK land.
  const isSectorRoute = segments[0] === "fleets";
  const sectorId = isSectorRoute ? segments[1] : undefined;
  const sectorBase = sectorId ? `/fleets/${sectorId}` : undefined;
  const section = sectorId ? segments[2] : undefined;
  const gatewayId = sectorId ? segments[3] : undefined;

  useEffect(() => {
    if (previousPath.current === pathname) {
      return;
    }

    previousPath.current = pathname;
    const pageHeading = document.querySelector<HTMLElement>("#main-content h1");
    if (pageHeading) {
      pageHeading.tabIndex = -1;
      pageHeading.focus();
    }
  }, [pathname]);

  const skipToContent = (
    <SkipToContent href="#main-content">
      <FormattedMessage {...messages.skipToContent} />
    </SkipToContent>
  );

  const masthead = (
    <Masthead>
      <MastheadMain>
        <MastheadToggle>
          <PageToggleButton
            aria-label={intl.formatMessage(messages.navigationToggle)}
            id="primary-navigation-toggle"
            isHamburgerButton
            variant="plain"
          />
        </MastheadToggle>
        <MastheadBrand>
          <MastheadLogo
            className={styles.brand}
            component={Link}
            {...{ to: "/" }}
          >
            <span aria-hidden="true" className={styles.brandMark} />
            <FormattedMessage {...messages.productName} />
          </MastheadLogo>
        </MastheadBrand>
      </MastheadMain>
      <MastheadContent>
        <Toolbar hasNoPadding isFullHeight isStatic>
          <ToolbarContent>
            <ToolbarGroup align={{ default: "alignEnd" }}>
              <ToolbarItem>
                <Label color="red" isCompact>
                  <FormattedMessage {...messages.developmentPreview} />
                </Label>
              </ToolbarItem>
            </ToolbarGroup>
          </ToolbarContent>
        </Toolbar>
      </MastheadContent>
    </Masthead>
  );

  const sidebar = (
    <PageSidebar>
      <PageSidebarBody>
        <Nav aria-label={intl.formatMessage(messages.primaryNavigation)}>
          <NavList>
            <NavigationItem
              isActive={pathname === "/"}
              label={<FormattedMessage {...messages.overview} />}
              to="/"
            />
            <NavigationItem
              isActive={pathname === "/fleets"}
              label={<FormattedMessage {...messages.sectors} />}
              to="/fleets"
            />
          </NavList>
          {sectorBase ? (
            <NavGroup title={intl.formatMessage(messages.selectedSector)}>
              <NavigationItem
                isActive={pathname === sectorBase}
                label={<FormattedMessage {...messages.sectorOverview} />}
                to={sectorBase}
              />
              <NavigationItem
                isActive={section === "gateways"}
                label={<FormattedMessage {...messages.gateways} />}
                to={`${sectorBase}/gateways`}
              />
              <NavigationItem
                isActive={section === "settings"}
                label={<FormattedMessage {...messages.settings} />}
                to={`${sectorBase}/settings`}
              />
            </NavGroup>
          ) : null}
        </Nav>
      </PageSidebarBody>
    </PageSidebar>
  );

  let breadcrumb: React.ReactNode;
  if (isSectorRoute) {
    breadcrumb = (
      <Breadcrumb aria-label={intl.formatMessage(messages.breadcrumbLabel)}>
        <BreadcrumbItem isActive={!sectorId} to="/fleets">
          <FormattedMessage {...messages.sectors} />
        </BreadcrumbItem>
        {sectorBase ? (
          <BreadcrumbItem isActive={!section} to={sectorBase}>
            <FormattedMessage
              {...messages.sectorContext}
              values={{ sectorId }}
            />
          </BreadcrumbItem>
        ) : null}
        {section === "gateways" && sectorBase ? (
          <BreadcrumbItem isActive={!gatewayId} to={`${sectorBase}/gateways`}>
            <FormattedMessage {...messages.gateways} />
          </BreadcrumbItem>
        ) : null}
        {section === "gateways" && gatewayId ? (
          <BreadcrumbItem isActive>
            <FormattedMessage
              {...messages.gatewayContext}
              values={{ gatewayId }}
            />
          </BreadcrumbItem>
        ) : null}
        {section === "settings" ? (
          <BreadcrumbItem isActive>
            <FormattedMessage {...messages.settings} />
          </BreadcrumbItem>
        ) : null}
        {section === "clients" ? (
          <BreadcrumbItem isActive>
            <FormattedMessage {...messages.clients} />
          </BreadcrumbItem>
        ) : null}
        {section === "keys" ? (
          <BreadcrumbItem isActive>
            <FormattedMessage {...messages.keys} />
          </BreadcrumbItem>
        ) : null}
      </Breadcrumb>
    );
  }

  return (
    <Page
      banner={
        sectorId ? (
          <SectorContextBar pathname={pathname} selectedSectorId={sectorId} />
        ) : undefined
      }
      breadcrumb={breadcrumb}
      defaultManagedSidebarIsOpen
      isContentFilled
      isManagedSidebar
      mainContainerId="main-content"
      masthead={masthead}
      sidebar={sidebar}
      skipToContent={skipToContent}
    >
      <Outlet />
    </Page>
  );
}
