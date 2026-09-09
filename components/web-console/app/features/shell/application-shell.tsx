import {
  Breadcrumb,
  BreadcrumbItem,
  Button,
  Masthead,
  MastheadBrand,
  MastheadContent,
  MastheadLogo,
  MastheadMain,
  Page,
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
import { messages } from "../../i18n/messages";
import productLogo from "../../../../../images/brand/logo.png";
import { useColorScheme } from "./use-color-scheme";
import { useRouteHeadingFocus } from "./route-focus";
import { UserMenu } from "./user-menu";
import styles from "./application-shell.module.css";

export function ApplicationShell() {
  const intl = useIntl();
  const { pathname } = useLocation();
  const navigate = useNavigate();
  const gatewayNavigation = useMemo<GatewayUiNavigation>(
    () => ({
      collectionHref: "/",
      createHref: "/gateways/new",
      detailHref: (gatewayId) => `/gateways/${encodeURIComponent(gatewayId)}`,
      navigate: (href, options) => navigate(href, options),
    }),
    [navigate],
  );
  const dashboardNavigation = useMemo<DashboardUiNavigation>(
    () => ({
      collectionHref: "/",
      navigate: (href) => navigate(href),
    }),
    [navigate],
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

  const skipToContent = (
    <SkipToContent href="#main-content">
      <FormattedMessage {...messages.skipToContent} />
    </SkipToContent>
  );

  const masthead = (
    <Masthead>
      <MastheadMain>
        <MastheadBrand>
          <MastheadLogo
            className={styles.brand}
            component={Link}
            {...{ to: "/" }}
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
              <UserMenu />
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
        <BreadcrumbItem to="/">
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

  return (
    <DashboardUiProvider
      dashboard={dashboardOperations}
      navigation={dashboardNavigation}
    >
      <GatewayUiProvider
        gateways={gatewayOperations}
        navigation={gatewayNavigation}
      >
        <Page
          breadcrumb={breadcrumb}
          isContentFilled
          mainContainerId="main-content"
          masthead={masthead}
          skipToContent={skipToContent}
        >
          <Outlet />
        </Page>
      </GatewayUiProvider>
    </DashboardUiProvider>
  );
}
