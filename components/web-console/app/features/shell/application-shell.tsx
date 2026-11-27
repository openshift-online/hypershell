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
  ToolbarItem,
} from "@patternfly/react-core";
import { MoonIcon, SunIcon } from "@patternfly/react-icons";
import {
  gatewayMessages,
  gatewayQueryKey,
  GatewayUiProvider,
  type GatewayUiNavigation,
} from "@openshift-online/hypershell-gateway-management-ui";
import {
  DashboardUiProvider,
  type DashboardUiNavigation,
} from "@openshift-online/hypershell-operational-dashboard-ui";
import { useQuery } from "@tanstack/react-query";
import { useMemo } from "react";
import { FormattedMessage, useIntl } from "react-intl";
import { Link, Outlet, useLocation, useNavigate } from "react-router";

import { dashboardOperations } from "../../composition/dashboard-composition";
import { gatewayOperations } from "../../composition/gateway-composition";
import { hasDashboardAdminRole } from "../../lib/session-roles";
import { messages } from "../../i18n/messages";
import productLogo from "../../../../../images/brand/logo.png";
import { platformPoliciesMessages } from "../platform-policies/platform-policies-messages";
import { TerminalProvider } from "../terminal/terminal-context";
import { TerminalDrawer } from "../terminal/terminal-drawer";
import { TerminalTrigger } from "../terminal/terminal-trigger";
import { usePlatformPolicy } from "../platform-policies/use-platform-policies";
import { usePlatformPolicyAccess } from "../platform-policies/use-platform-policy-access";
import { useColorScheme } from "./use-color-scheme";
import { useRouteHeadingFocus } from "./route-focus";
import { useSession } from "./use-session";
import { SignInAction } from "./sign-in-action";
import { UserMenu } from "./user-menu";
import {
  gatewayDetailPath,
  platformPolicyDetailPath,
  routePaths,
} from "../../../shared/routes";
import styles from "./application-shell.module.css";

export function ApplicationShell() {
  const intl = useIntl();
  const { data: session } = useSession();
  const { pathname } = useLocation();
  const navigate = useNavigate();
  const gatewayNavigation = useMemo<GatewayUiNavigation>(
    () => ({
      collectionHref: routePaths.gatewayList,
      createHref: routePaths.gatewayNew,
      detailHref: gatewayDetailPath,
      navigate: (href, options) => navigate(href, options),
    }),
    [navigate],
  );
  const dashboardNavigation = useMemo<DashboardUiNavigation>(
    () => ({
      adoptionHref: routePaths.dashboardAdoption,
      collectionHref: routePaths.gatewayList,
      dashboardHref: routePaths.dashboard,
      inventoryHref: routePaths.dashboardInventory,
      provisioningHref: routePaths.dashboardProvisioning,
      reliabilityHref: routePaths.dashboardReliability,
      usageHref: routePaths.dashboardUsage,
      navigate: (href) => navigate(href),
    }),
    [navigate],
  );
  const gatewayCurrentUser = useMemo(
    () => ({
      isPlatformAdmin: hasDashboardAdminRole(session?.roles ?? []),
      subject: session?.user?.sub,
      username: session?.user?.preferredUsername,
    }),
    [session?.roles, session?.user?.preferredUsername, session?.user?.sub],
  );
  const { scheme, toggle: toggleColorScheme } = useColorScheme();
  useRouteHeadingFocus(pathname);
  const segments = pathname.split("/").filter(Boolean);
  const gatewaySegment = segments[0] === "gateways" ? segments[1] : undefined;
  const isNewGateway = gatewaySegment === "new";
  const gatewayId = isNewGateway ? undefined : gatewaySegment;
  const gatewayQuery = useQuery({
    enabled: gatewayId !== undefined,
    queryFn: ({ signal }) =>
      gatewayOperations.getGateway(gatewayId ?? "", signal),
    queryKey: gatewayQueryKey(gatewayId ?? ""),
  });
  // Policy administration is visible only to platform-policy:admin or
  // platform:admin holders (WEB-PADM-01). The API remains the enforcement point.
  const platformPolicyAccess = usePlatformPolicyAccess();
  const isPolicyAdminRoute =
    segments[0] === "admin" && segments[1] === "platform-policies";
  const policySegment = isPolicyAdminRoute ? segments[2] : undefined;
  const isNewPolicy = policySegment === "new";
  const isEditPolicy = isPolicyAdminRoute && segments[3] === "edit";
  const policyId =
    policySegment && !isNewPolicy && platformPolicyAccess.isAuthorized
      ? policySegment
      : "";
  const policyQuery = usePlatformPolicy(policyId);

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
            aria-label={intl.formatMessage(messages.toggleNavigation)}
            isHamburgerButton
          />
        </MastheadToggle>
        <MastheadBrand>
          <MastheadLogo
            className={styles.brand}
            component={Link}
            {...{ to: routePaths.home }}
          >
            <img
              alt=""
              aria-hidden="true"
              className={styles.brandLogo}
              src={productLogo}
            />
            <FormattedMessage {...messages.productName} />
          </MastheadLogo>
        </MastheadBrand>
      </MastheadMain>
      <MastheadContent>
        <Toolbar isStatic>
          <ToolbarContent>
            <ToolbarItem align={{ default: "alignEnd" }}>
              <Button
                aria-label={intl.formatMessage(
                  scheme === "dark"
                    ? messages.switchToLightMode
                    : messages.switchToDarkMode,
                )}
                icon={scheme === "dark" ? <SunIcon /> : <MoonIcon />}
                onClick={toggleColorScheme}
                variant="plain"
              />
            </ToolbarItem>
            <ToolbarItem>
              <TerminalTrigger />
            </ToolbarItem>
            <ToolbarItem>
              <UserMenu />
              <SignInAction />
            </ToolbarItem>
          </ToolbarContent>
        </Toolbar>
      </MastheadContent>
    </Masthead>
  );

  let breadcrumb: React.ReactNode;
  if (gatewayId || isNewGateway) {
    breadcrumb = (
      <Breadcrumb aria-label={intl.formatMessage(messages.breadcrumbLabel)}>
        <BreadcrumbItem to={routePaths.gatewayList}>
          <FormattedMessage {...gatewayMessages.gateways} />
        </BreadcrumbItem>
        {gatewayId ? (
          <BreadcrumbItem isActive>
            {gatewayQuery.data?.name ??
              intl.formatMessage(gatewayMessages.gateway)}
          </BreadcrumbItem>
        ) : null}
        {isNewGateway ? (
          <BreadcrumbItem isActive>
            <FormattedMessage {...gatewayMessages.provisionGateway} />
          </BreadcrumbItem>
        ) : null}
      </Breadcrumb>
    );
  }

  if (
    isPolicyAdminRoute &&
    policySegment &&
    platformPolicyAccess.isAuthorized
  ) {
    breadcrumb = (
      <Breadcrumb aria-label={intl.formatMessage(messages.breadcrumbLabel)}>
        <BreadcrumbItem to={routePaths.adminPlatformPolicies}>
          {intl.formatMessage(platformPoliciesMessages.navPlatformPolicies)}
        </BreadcrumbItem>
        {isNewPolicy ? (
          <BreadcrumbItem isActive>
            {intl.formatMessage(platformPoliciesMessages.breadcrumbCreate)}
          </BreadcrumbItem>
        ) : null}
        {!isNewPolicy && isEditPolicy ? (
          <BreadcrumbItem to={platformPolicyDetailPath(policySegment)}>
            {policyQuery.data?.name ??
              intl.formatMessage(platformPoliciesMessages.breadcrumbPolicy)}
          </BreadcrumbItem>
        ) : null}
        {!isNewPolicy ? (
          <BreadcrumbItem isActive>
            {isEditPolicy
              ? intl.formatMessage(platformPoliciesMessages.breadcrumbEdit)
              : (policyQuery.data?.name ??
                intl.formatMessage(platformPoliciesMessages.breadcrumbPolicy))}
          </BreadcrumbItem>
        ) : null}
      </Breadcrumb>
    );
  }

  // Determine active navigation item based on current route
  const isGatewayRoute =
    pathname.startsWith(routePaths.gatewayList) ||
    pathname === routePaths.gatewayList;
  const isDashboardRoute = pathname.startsWith(routePaths.dashboard);
  const isOverviewRoute = pathname === routePaths.home;
  const isDocsRoute =
    pathname === routePaths.docs || pathname.startsWith(`${routePaths.docs}/`);

  const sidebar = (
    <PageSidebar>
      <PageSidebarBody>
        <Nav aria-label={intl.formatMessage(messages.primaryNavigation)}>
          <NavList>
            <NavItem isActive={isOverviewRoute} key={routePaths.home}>
              <Link className={styles.navLink} to={routePaths.home}>
                {intl.formatMessage(messages.overview)}
              </Link>
            </NavItem>
            <NavItem isActive={isGatewayRoute} key={routePaths.gatewayList}>
              <Link className={styles.navLink} to={routePaths.gatewayList}>
                {intl.formatMessage(gatewayMessages.gateways)}
              </Link>
            </NavItem>
            <NavItem isActive={isDashboardRoute} key={routePaths.dashboard}>
              <Link className={styles.navLink} to={routePaths.dashboard}>
                {intl.formatMessage(messages.dashboard)}
              </Link>
            </NavItem>
            <NavItem isActive={isDocsRoute} key={routePaths.docs}>
              <Link className={styles.navLink} to={routePaths.docs}>
                {intl.formatMessage(messages.documentation)}
              </Link>
            </NavItem>
          </NavList>
          {platformPolicyAccess.isAuthorized ? (
            <NavGroup
              title={intl.formatMessage(
                platformPoliciesMessages.navAdministration,
              )}
            >
              <NavItem
                isActive={isPolicyAdminRoute}
                key={routePaths.adminPlatformPolicies}
              >
                <Link
                  className={styles.navLink}
                  to={routePaths.adminPlatformPolicies}
                >
                  {intl.formatMessage(
                    platformPoliciesMessages.navPlatformPolicies,
                  )}
                </Link>
              </NavItem>
            </NavGroup>
          ) : null}
        </Nav>
      </PageSidebarBody>
    </PageSidebar>
  );

  return (
    <TerminalProvider>
      <DashboardUiProvider
        dashboard={dashboardOperations}
        isAdmin={hasDashboardAdminRole(session?.roles ?? [])}
        navigation={dashboardNavigation}
      >
        <GatewayUiProvider
          currentUser={gatewayCurrentUser}
          gateways={gatewayOperations}
          navigation={gatewayNavigation}
        >
          <TerminalDrawer>
            <Page
              breadcrumb={breadcrumb}
              isContentFilled
              isManagedSidebar
              mainContainerId="main-content"
              masthead={masthead}
              sidebar={sidebar}
              skipToContent={skipToContent}
            >
              <Outlet />
            </Page>
          </TerminalDrawer>
        </GatewayUiProvider>
      </DashboardUiProvider>
    </TerminalProvider>
  );
}
