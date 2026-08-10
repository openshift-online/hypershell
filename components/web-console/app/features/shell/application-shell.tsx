import {
  Breadcrumb,
  BreadcrumbItem,
  Masthead,
  MastheadBrand,
  MastheadLogo,
  MastheadMain,
  Page,
  SkipToContent,
} from "@patternfly/react-core";
import {
  gatewayMessages,
  gatewayQueryKey,
  GatewayUiProvider,
  type GatewayUiNavigation,
} from "@openshift-online/hypershell-gateway-management-ui";
import { useQuery } from "@tanstack/react-query";
import { useMemo } from "react";
import { FormattedMessage, useIntl } from "react-intl";
import { Link, Outlet, useLocation, useNavigate } from "react-router";

import { gatewayOperations } from "../../composition/gateway-composition";
import { messages } from "../../i18n/messages";
import productLogo from "../../../../../images/brand/logo.png";
import { useRouteHeadingFocus } from "./route-focus";
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
  );
}
