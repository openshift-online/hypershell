import { defineMessages } from "react-intl";

export const messages = defineMessages({
  account: {
    id: "app.account",
    defaultMessage: "Account",
    description: "Fallback label for the identity menu when no name is known.",
  },
  consoleVersion: {
    id: "app.version.console",
    defaultMessage: "Console build: {version}",
    description:
      "Web console build version shown in the identity menu. The version is the source commit the console image was built from.",
  },
  apiVersion: {
    id: "app.version.api",
    defaultMessage: "API build: {version}",
    description:
      "API server build version shown in the identity menu. The version is the source commit the API server image was built from.",
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
  dashboardReliabilityNav: {
    id: "app.nav.dashboardReliability",
    defaultMessage: "Reliability dashboard",
    description:
      "Page and navigation label for the HyperShell reliability dashboard.",
  },
  dashboardReliabilityPageDescription: {
    id: "app.page.dashboardReliability.description",
    defaultMessage:
      "API reliability metrics dashboard for HyperShell request rate, error rate, and latency.",
    description:
      "Browser metadata description for the reliability dashboard page.",
  },
  dashboardAdoptionNav: {
    id: "app.nav.dashboardAdoption",
    defaultMessage: "Adoption dashboard",
    description:
      "Page and navigation label for the HyperShell adoption dashboard.",
  },
  dashboardAdoptionPageDescription: {
    id: "app.page.dashboardAdoption.description",
    defaultMessage:
      "Adoption metrics dashboard for HyperShell registrations, activity, conversion, and retention.",
    description:
      "Browser metadata description for the adoption dashboard page.",
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
  homepageTitle: {
    id: "app.page.homepage.title",
    defaultMessage: "HyperShell homepage",
    description: "Browser metadata title for the HyperShell homepage.",
  },
  homepageMetaDescription: {
    id: "app.page.homepage.description",
    defaultMessage:
      "Gateway health, quick gateway creation, dashboards, support, and learning resources.",
    description: "Browser metadata description for the HyperShell homepage.",
  },
  homepageWelcomeTitle: {
    id: "app.homepage.welcome.title",
    defaultMessage: "Pick a gateway. Ship your sandboxes.",
    description: "Primary heading on the HyperShell homepage.",
  },
  homepageWelcomeBody: {
    id: "app.homepage.welcome.body",
    defaultMessage:
      "HyperShell runs OpenShell gateways across clusters and clouds for you. Choose where your workload belongs and get to a running gateway in just a few steps.",
    description: "Primary call-to-action copy on the HyperShell homepage.",
  },
  homepageCreateGateway: {
    id: "app.homepage.actions.createGateway",
    defaultMessage: "Create a gateway",
    description: "Primary action that opens the full gateway creation page.",
  },
  homepageViewGateways: {
    id: "app.homepage.actions.viewGateways",
    defaultMessage: "View gateways",
    description: "Action that opens the gateway list.",
  },
  homepageQuickCreate: {
    id: "app.homepage.actions.quickCreate",
    defaultMessage: "Quick create",
    description: "Action that opens the gateway quick-create dialog.",
  },
  homepageViewMyGateways: {
    id: "app.homepage.actions.viewMyGateways",
    defaultMessage: "View my gateways",
    description: "Action in the gateway health card.",
  },
  homepageDashboard: {
    id: "app.homepage.actions.dashboard",
    defaultMessage: "Dashboard",
    description: "Action that opens the operational dashboard.",
  },
  homepageGoToDashboard: {
    id: "app.homepage.actions.goToDashboard",
    defaultMessage: "Go to your dashboard",
    description: "Action in the non-administrator dashboard card.",
  },
  homepageQuickCreateTitle: {
    id: "app.homepage.quickCreate.title",
    defaultMessage: "Quick create a gateway",
    description: "Title of the gateway quick-create dialog.",
  },
  homepageQuickCreateBody: {
    id: "app.homepage.quickCreate.body",
    defaultMessage:
      "Quick create makes a gateway on IBM Cloud public with a generated name. You can rename it after it starts.",
    description: "Explanation shown in the gateway quick-create dialog.",
  },
  homepageQuickCreateSubmit: {
    id: "app.homepage.quickCreate.submit",
    defaultMessage: "Create gateway",
    description: "Confirmation action in the quick-create dialog.",
  },
  homepageCancel: {
    id: "app.homepage.quickCreate.cancel",
    defaultMessage: "Cancel",
    description: "Cancellation action in the quick-create dialog.",
  },
  homepageQuickCreateError: {
    id: "app.homepage.quickCreate.error",
    defaultMessage:
      "The gateway could not be created. Retry to generate another unique name.",
    description: "Error shown when quick create cannot create a gateway.",
  },
  homepageQuickCreateLocalKindBody: {
    id: "app.homepage.quickCreate.localKindBody",
    defaultMessage:
      "Quick create makes a gateway on local-kind with a generated name. You can rename it after it starts.",
    description: "Explanation shown in the local-kind quick-create dialog.",
  },
  homepageGatewayHealthTitle: {
    id: "app.homepage.gatewayHealth.title",
    defaultMessage: "Gateway health",
    description: "Title of the gateway health card.",
  },
  homepageGatewayHealthUnavailable: {
    id: "app.homepage.gatewayHealth.unavailable",
    defaultMessage: "Gateway status is unavailable",
    description: "Alert title when gateway health cannot be loaded.",
  },
  homepageDataUnavailableBody: {
    id: "app.homepage.dataUnavailable.body",
    defaultMessage: "Refresh the page to try again.",
    description: "Recovery guidance shown when homepage data is unavailable.",
  },
  homepageAllGatewaysHealthy: {
    id: "app.homepage.gatewayHealth.allHealthy",
    defaultMessage:
      "{total, plural, =0 {No gateways yet} one {# gateway is healthy} other {All # gateways are healthy}}",
    description: "Alert title for the visible gateway health summary.",
  },
  homepageGatewaysNeedAttention: {
    id: "app.homepage.gatewayHealth.attention",
    defaultMessage: "{count} gateways need attention",
    description: "Alert title when gateways need attention.",
  },
  homepageGatewayProgress: {
    id: "app.homepage.gatewayHealth.progress",
    defaultMessage: "{healthy} of {total} gateways healthy",
    description: "Progress label for healthy gateway count.",
  },
  homepageLoading: {
    id: "app.homepage.loading",
    defaultMessage: "Loading",
    description: "Accessible label for homepage loading states.",
  },
  homepageYourDashboardTitle: {
    id: "app.homepage.dashboard.title",
    defaultMessage: "Your dashboard",
    description: "Title of the non-administrator dashboard card.",
  },
  homepageYourDashboardBody: {
    id: "app.homepage.dashboard.body",
    defaultMessage:
      "Track your gateways, sandbox activity, and reliability in one place.",
    description: "Body of the non-administrator dashboard card.",
  },
  homepageRoleLoadingTitle: {
    id: "app.homepage.roleLoading.title",
    defaultMessage: "Dashboard",
    description: "Neutral title shown while the session role is loading.",
  },
  homepagePlatformOverviewTitle: {
    id: "app.homepage.platformOverview.title",
    defaultMessage: "Platform overview",
    description: "Title of the administrator platform overview card.",
  },
  homepagePlatformOverviewUnavailable: {
    id: "app.homepage.platformOverview.unavailable",
    defaultMessage: "Platform overview is unavailable",
    description: "Alert title when platform overview cannot be loaded.",
  },
  homepageGatewaysChartTitle: {
    id: "app.homepage.platformOverview.gateways",
    defaultMessage: "Gateways",
    description: "Title of the gateway status donut chart.",
  },
  homepageSandboxesChartTitle: {
    id: "app.homepage.platformOverview.sandboxes",
    defaultMessage: "Active sandboxes",
    description: "Title of the active sandbox status donut chart.",
  },
  homepageGatewaysSubTitle: {
    id: "app.homepage.platformOverview.gatewaysSubTitle",
    defaultMessage: "gateways",
    description: "Center subtitle for the gateway status donut chart.",
  },
  homepageSandboxesSubTitle: {
    id: "app.homepage.platformOverview.sandboxesSubTitle",
    defaultMessage: "sandboxes",
    description: "Center subtitle for the active sandbox status donut chart.",
  },
  homepageClusterAvailabilityTitle: {
    id: "app.homepage.platformOverview.clusters.title",
    defaultMessage: "Cluster availability",
    description: "Title of the cluster availability section.",
  },
  homepageClusterProgress: {
    id: "app.homepage.platformOverview.clusters.progress",
    defaultMessage: "{healthy} of {total} clusters healthy",
    description: "Progress label for healthy cluster count.",
  },
  homepageClusterUnavailableStatus: {
    id: "app.homepage.platformOverview.clusters.unavailable",
    defaultMessage: "Unavailable",
    description: "Status shown for an unhealthy cluster.",
  },
  homepageLearnTitle: {
    id: "app.homepage.learn.title",
    defaultMessage: "Learn and explore",
    description: "Title of the learning links card.",
  },
  homepageHyperShellRepository: {
    id: "app.homepage.learn.repository",
    defaultMessage: "HyperShell repo",
    description: "External link to the HyperShell repository.",
  },
  homepageHyperShellRepositoryDescription: {
    id: "app.homepage.learn.repository.description",
    defaultMessage: "Explore the source and project history.",
    description: "Description for the HyperShell repository link.",
  },
  homepageOpenShellDocs: {
    id: "app.homepage.learn.docs",
    defaultMessage: "OpenShell docs",
    description: "External link to the OpenShell documentation.",
  },
  homepageOpenShellDocsDescription: {
    id: "app.homepage.learn.docs.description",
    defaultMessage: "Get oriented with OpenShell concepts.",
    description: "Description for the OpenShell documentation link.",
  },
  homepageOpenShellCli: {
    id: "app.homepage.learn.cli",
    defaultMessage: "OpenShell CLI",
    description:
      "External link to the OpenShell CLI installation instructions.",
  },
  homepageOpenShellCliDescription: {
    id: "app.homepage.learn.cli.description",
    defaultMessage: "Install and start using the CLI.",
    description: "Description for the OpenShell CLI installation link.",
  },
  homepageSupportTitle: {
    id: "app.homepage.support.title",
    defaultMessage: "Talk to the team in #forum-hypershell",
    description: "Title for the homepage support channel card.",
  },
  homepageSupportDescription: {
    id: "app.homepage.support.description",
    defaultMessage:
      "Onboarding, access to a gateway, questions, or something broken. We answer in the channel.",
    description: "Description for the homepage support channel card.",
  },
  homepageSupportOpenAction: {
    id: "app.homepage.support.open",
    defaultMessage: "Open in Slack",
    description: "Action that opens the HyperShell Slack channel.",
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
