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
import { useQuery } from "@tanstack/react-query";
import { FormattedMessage, useIntl } from "react-intl";
import { Link, Outlet, useLocation } from "react-router";

import { messages } from "../../i18n/messages";
import { gatewayQueryKey, getGateway } from "../gateways/gateway-data";
import productLogo from "../../../../../images/brand/logo.png";
import { useRouteHeadingFocus } from "./route-focus";
import styles from "./application-shell.module.css";

export function ApplicationShell() {
  const intl = useIntl();
  const { pathname } = useLocation();
  useRouteHeadingFocus(pathname);
  const segments = pathname.split("/").filter(Boolean);
  const gatewaySegment = segments[0] === "gateways" ? segments[1] : undefined;
  const isNewGateway = gatewaySegment === "new";
  const gatewayId = isNewGateway ? undefined : gatewaySegment;
  const gatewayQuery = useQuery({
    enabled: gatewayId !== undefined,
    queryFn: ({ signal }) => getGateway(gatewayId ?? "", signal),
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
          <FormattedMessage {...messages.gateways} />
        </BreadcrumbItem>
        {gatewayId ? (
          <BreadcrumbItem isActive>
            {gatewayQuery.data?.name ?? intl.formatMessage(messages.gateway)}
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
      isContentFilled
      mainContainerId="main-content"
      masthead={masthead}
      skipToContent={skipToContent}
    >
      <Outlet />
    </Page>
  );
}
