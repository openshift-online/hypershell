import {
  Card,
  CardBody,
  CardTitle,
  EmptyState,
  EmptyStateBody,
  Gallery,
  PageSection,
  Spinner,
  Title,
} from "@patternfly/react-core";
import { useQuery } from "@tanstack/react-query";
import { defineMessages, FormattedMessage, useIntl } from "react-intl";

import {
  fetchGatewayMetrics,
  gatewayMetricsQueryKey,
  gatewayPhases,
  type GatewayPhaseCounts,
} from "./gateway-metrics-data";

const messages = defineMessages({
  dashboardTitle: {
    id: "app.metrics.dashboard.title",
    defaultMessage: "Gateway metrics",
    description: "Heading for the gateway metrics dashboard page.",
  },
  loadError: {
    id: "app.metrics.dashboard.loadError",
    defaultMessage: "Metrics could not be loaded",
    description: "Shown when the metrics endpoint returns an error.",
  },
  loadErrorBody: {
    id: "app.metrics.dashboard.loadErrorBody",
    defaultMessage:
      "Check that Prometheus is running and reachable, then refresh.",
    description: "Recovery guidance shown below the metrics load error title.",
  },
  loadingAriaLabel: {
    id: "app.metrics.dashboard.loading",
    defaultMessage: "Loading metrics",
    description: "Accessible label for the metrics loading spinner.",
  },
  phaseRunning: {
    id: "app.metrics.phase.running",
    defaultMessage: "Running",
    description: "Label for the Running gateway phase metric card.",
  },
  phaseProvisioning: {
    id: "app.metrics.phase.provisioning",
    defaultMessage: "Provisioning",
    description: "Label for the Provisioning gateway phase metric card.",
  },
  phaseDegraded: {
    id: "app.metrics.phase.degraded",
    defaultMessage: "Degraded",
    description: "Label for the Degraded gateway phase metric card.",
  },
  phaseFailed: {
    id: "app.metrics.phase.failed",
    defaultMessage: "Failed",
    description: "Label for the Failed gateway phase metric card.",
  },
  gateways: {
    id: "app.metrics.phase.gateways",
    defaultMessage: "{count, plural, one {# gateway} other {# gateways}}",
    description: "Pluralized gateway count shown inside each phase card.",
  },
});

const phaseMessageKey = {
  Running: messages.phaseRunning,
  Provisioning: messages.phaseProvisioning,
  Degraded: messages.phaseDegraded,
  Failed: messages.phaseFailed,
} as const satisfies Record<keyof GatewayPhaseCounts, { id: string }>;

const phaseColor: Record<keyof GatewayPhaseCounts, string> = {
  Running: "var(--pf-t--global--color--status--success--default)",
  Provisioning: "var(--pf-t--global--color--status--info--default)",
  Degraded: "var(--pf-t--global--color--status--warning--default)",
  Failed: "var(--pf-t--global--color--status--danger--default)",
};

function PhaseCard({
  phase,
  count,
}: {
  phase: keyof GatewayPhaseCounts;
  count: number;
}) {
  const intl = useIntl();
  return (
    <Card>
      <CardTitle>
        <span style={{ color: phaseColor[phase] }}>
          {intl.formatMessage(phaseMessageKey[phase])}
        </span>
      </CardTitle>
      <CardBody>
        <Title headingLevel="h3" size="4xl">
          {count}
        </Title>
        <FormattedMessage {...messages.gateways} values={{ count }} />
      </CardBody>
    </Card>
  );
}

export function GatewayMetricsDashboard() {
  const intl = useIntl();
  const { data, isLoading, isError } = useQuery({
    queryKey: gatewayMetricsQueryKey,
    queryFn: ({ signal }) => fetchGatewayMetrics(signal),
    refetchInterval: 30_000,
  });

  if (isLoading) {
    return (
      <PageSection>
        <Spinner aria-label={intl.formatMessage(messages.loadingAriaLabel)} />
      </PageSection>
    );
  }

  if (isError || !data) {
    return (
      <PageSection>
        <EmptyState>
          <EmptyStateBody>
            <Title headingLevel="h2" size="lg">
              <FormattedMessage {...messages.loadError} />
            </Title>
            <p>
              <FormattedMessage {...messages.loadErrorBody} />
            </p>
          </EmptyStateBody>
        </EmptyState>
      </PageSection>
    );
  }

  return (
    <PageSection>
      <Title headingLevel="h1" size="xl">
        <FormattedMessage {...messages.dashboardTitle} />
      </Title>
      <Gallery hasGutter minWidths={{ default: "200px" }}>
        {gatewayPhases.map((phase) => (
          <PhaseCard key={phase} phase={phase} count={data[phase]} />
        ))}
      </Gallery>
    </PageSection>
  );
}
