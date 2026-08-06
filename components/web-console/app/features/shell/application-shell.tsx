import {
  Breadcrumb,
  BreadcrumbItem,
  Button,
  Masthead,
  MastheadBrand,
  MastheadContent,
  MastheadLogo,
  MastheadMain,
  MastheadToggle,
  Nav,
  NavExpandable,
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
import productLogo from "../../../../../images/brand/logo.png";
import styles from "./application-shell.module.css";

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

export function AdminShell() {
  const intl = useIntl();
  const { pathname } = useLocation();
  const previousPath = useRef(pathname);
  const segments = pathname.split("/").filter(Boolean);
  const section = segments[0] === "admin" ? segments[1] : undefined;
  const gatewaySegment = section === "gateways" ? segments[2] : undefined;
  const isNewGateway = gatewaySegment === "new";
  const gatewayId = isNewGateway ? undefined : gatewaySegment;

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
            {...{ to: "/admin" }}
          >
            <img
              alt=""
              aria-hidden="true"
              className={styles.brandLogo}
              src={productLogo}
            />
            <FormattedMessage {...messages.adminProductName} />
          </MastheadLogo>
        </MastheadBrand>
      </MastheadMain>
      <MastheadContent>
        <Toolbar hasNoPadding isFullHeight isStatic>
          <ToolbarContent>
            <ToolbarGroup align={{ default: "alignEnd" }}>
              <ToolbarItem>
                <Button component={Link} variant="secondary" {...{ to: "/" }}>
                  <FormattedMessage {...messages.gatewayDirectory} />
                </Button>
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
            <NavExpandable
              groupId="administration"
              isExpanded
              title={intl.formatMessage(messages.administration)}
            >
              <NavigationItem
                isActive={pathname === "/admin"}
                label={<FormattedMessage {...messages.overview} />}
                to="/admin"
              />
              <NavigationItem
                isActive={section === "clusters"}
                label={<FormattedMessage {...messages.clusters} />}
                to="/admin/clusters"
              />
              <NavigationItem
                isActive={section === "gateways"}
                label={<FormattedMessage {...messages.gateways} />}
                to="/admin/gateways"
              />
            </NavExpandable>
          </NavList>
        </Nav>
      </PageSidebarBody>
    </PageSidebar>
  );

  let breadcrumb: React.ReactNode;
  if (section) {
    breadcrumb = (
      <Breadcrumb aria-label={intl.formatMessage(messages.breadcrumbLabel)}>
        <BreadcrumbItem to="/admin">
          <FormattedMessage {...messages.administration} />
        </BreadcrumbItem>
        {section === "clusters" ? (
          <BreadcrumbItem isActive>
            <FormattedMessage {...messages.clusters} />
          </BreadcrumbItem>
        ) : null}
        {section === "gateways" ? (
          <BreadcrumbItem
            isActive={!gatewayId && !isNewGateway}
            to="/admin/gateways"
          >
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
        {isNewGateway ? (
          <BreadcrumbItem isActive>
            <FormattedMessage {...messages.provisionGateway} />
          </BreadcrumbItem>
        ) : null}
      </Breadcrumb>
    );
  }

  return (
    <Page
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
