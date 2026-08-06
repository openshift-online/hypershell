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
  ToolbarGroup,
  ToolbarItem,
} from "@patternfly/react-core";
import { FormattedMessage, useIntl } from "react-intl";
import { Link, Outlet, useLocation } from "react-router";

import { messages } from "../../i18n/messages";
import productLogo from "../../../../../images/brand/logo.png";
import { useRouteHeadingFocus } from "./route-focus";
import styles from "./application-shell.module.css";

export function AdminShell() {
  const intl = useIntl();
  const { pathname } = useLocation();
  useRouteHeadingFocus(pathname);
  const segments = pathname.split("/").filter(Boolean);
  const section = segments[0] === "admin" ? segments[1] : undefined;
  const gatewaySegment = section === "gateways" ? segments[2] : undefined;
  const isNewGateway = gatewaySegment === "new";
  const gatewayId = isNewGateway ? undefined : gatewaySegment;

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

  let breadcrumb: React.ReactNode;
  if (section === "gateways" && (gatewayId || isNewGateway)) {
    breadcrumb = (
      <Breadcrumb aria-label={intl.formatMessage(messages.breadcrumbLabel)}>
        <BreadcrumbItem to="/admin">
          <FormattedMessage {...messages.administration} />
        </BreadcrumbItem>
        {gatewayId ? (
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
      isContentFilled
      mainContainerId="main-content"
      masthead={masthead}
      skipToContent={skipToContent}
    >
      <Outlet />
    </Page>
  );
}
