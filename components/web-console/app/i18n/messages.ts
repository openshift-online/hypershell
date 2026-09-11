import { defineMessages } from "react-intl";

export const messages = defineMessages({
  account: {
    id: "app.account",
    defaultMessage: "Account",
    description: "Fallback label for the identity menu when no name is known.",
  },
  breadcrumbLabel: {
    id: "app.breadcrumb.ariaLabel",
    defaultMessage: "Breadcrumb",
    description: "Accessible label for the application breadcrumb navigation.",
  },
  dashboardNav: {
    id: "app.nav.dashboard",
    defaultMessage: "Operational dashboard",
    description: "Page and navigation label for the HyperShell dashboard.",
  },
  dashboardPageDescription: {
    id: "app.page.dashboard.description",
    defaultMessage:
      "Operational metrics dashboard for HyperShell adoption and provisioned resources.",
    description:
      "Browser metadata description for the operational dashboard page.",
  },
  dashboardAccessDeniedBody: {
    id: "app.page.dashboard.accessDenied.body",
    defaultMessage:
      "The operational dashboard is available only to HyperShell administrators.",
    description:
      "Recovery guidance shown when a signed-in user lacks the admin role for the dashboard.",
  },
  dashboardAccessDeniedTitle: {
    id: "app.page.dashboard.accessDenied.title",
    defaultMessage: "Access denied",
    description:
      "Heading shown when a signed-in user lacks the admin role for the dashboard.",
  },
  sessionLoadingLabel: {
    id: "app.session.loading.ariaLabel",
    defaultMessage: "Loading session",
    description: "Accessible label for the session loading spinner.",
  },
  errorBody: {
    id: "app.error.body",
    defaultMessage: "Refresh the page to try again.",
    description: "Recovery guidance shown after an unexpected route failure.",
  },
  errorTitle: {
    id: "app.error.title",
    defaultMessage: "The page could not be loaded",
    description: "Title shown after an unexpected route failure.",
  },
  helloDescription: {
    id: "app.hello.description",
    defaultMessage: "The HyperShell web console is ready for development.",
    description: "Supporting text on the initial web-console landing page.",
  },
  helloTitle: {
    id: "app.hello.title",
    defaultMessage: "Hello world",
    description: "Main heading on the initial web-console landing page.",
  },
  logout: {
    id: "app.logout",
    defaultMessage: "Log out",
    description: "Sign-out action in the masthead identity menu.",
  },
  notFoundBody: {
    id: "app.notFound.body",
    defaultMessage: "Check the address and try again.",
    description:
      "Recovery guidance shown when an application route does not exist.",
  },
  notFoundTitle: {
    id: "app.notFound.title",
    defaultMessage: "Page not found",
    description: "Heading shown when an application route does not exist.",
  },
  productName: {
    id: "app.productName",
    defaultMessage: "HyperShell",
    description: "HyperShell product name.",
  },
  skipToContent: {
    id: "app.skipToContent",
    defaultMessage: "Skip to content",
    description:
      "Accessibility link that moves focus to the main page content.",
  },
  switchToDarkMode: {
    id: "app.toggleDarkMode.off",
    defaultMessage: "Switch to dark mode",
    description:
      "Accessible label for the color scheme toggle when light mode is active.",
  },
  switchToLightMode: {
    id: "app.toggleDarkMode.on",
    defaultMessage: "Switch to light mode",
    description:
      "Accessible label for the color scheme toggle when dark mode is active.",
  },
  metricsNavLabel: {
    id: "app.nav.metrics",
    defaultMessage: "Metrics",
    description: "Navigation label for the gateway metrics dashboard page.",
  },
  metricsPageDescription: {
    id: "app.page.metrics.description",
    defaultMessage:
      "Gateway phase metrics dashboard showing running, provisioning, degraded, and failed gateway counts.",
    description: "Browser metadata description for the gateway metrics page.",
  },
});
