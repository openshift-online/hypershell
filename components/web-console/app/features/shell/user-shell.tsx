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
import styles from "./user-shell.module.css";

export function UserShell() {
  const intl = useIntl();
  const { pathname } = useLocation();
  useRouteHeadingFocus(pathname);
  const segments = pathname.split("/").filter(Boolean);
  const gatewayId = segments[0] === "gateways" ? segments[1] : undefined;

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
        <Toolbar hasNoPadding isFullHeight isStatic>
          <ToolbarContent>
            <ToolbarGroup align={{ default: "alignEnd" }}>
              <ToolbarItem>
                <Button
                  component={Link}
                  isInline
                  variant="link"
                  {...{ to: "/admin" }}
                >
                  <FormattedMessage {...messages.openAdministration} />
                </Button>
              </ToolbarItem>
            </ToolbarGroup>
          </ToolbarContent>
        </Toolbar>
      </MastheadContent>
    </Masthead>
  );

  const breadcrumb = gatewayId ? (
    <Breadcrumb aria-label={intl.formatMessage(messages.breadcrumbLabel)}>
      <BreadcrumbItem to="/">
        <FormattedMessage {...messages.gateways} />
      </BreadcrumbItem>
      <BreadcrumbItem isActive>{gatewayId}</BreadcrumbItem>
    </Breadcrumb>
  ) : undefined;

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
