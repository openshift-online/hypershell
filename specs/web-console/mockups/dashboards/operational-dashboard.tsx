import { Button, Content, PageSection, Title } from "@patternfly/react-core";
import type { ExtendedTemplateConfig } from "@patternfly/widgetized-dashboard";
import {
  CheckCircleIcon,
  ClusterIcon,
  CodeBranchIcon,
  ContainerNodeIcon,
  CubesIcon,
  ExclamationCircleIcon,
  PlusCircleIcon,
} from "@patternfly/react-icons";
import {
  DashboardFilters,
  DashboardNavigation,
  DashboardPurpose,
} from "../common/dashboard-controls";
import {
  DashboardDonut,
  DashboardSparkline,
  DashboardSummaryList,
  DashboardWidget,
  MockupWidgetizedLayout,
} from "../common/dashboard-widget";
import "./operational-dashboard.css";

// Deliberately mirrors the production dashboard contract without importing
// production layout or widget code. Mockups remain standalone design artifacts.
const operationalLayout: ExtendedTemplateConfig = {
  xl: [
    {
      h: 5,
      i: "resource-summary#1",
      title: "Resource summary",
      w: 2,
      x: 0,
      y: 0,
      widgetType: "resource-summary",
    },
    {
      h: 5,
      i: "gateway-status#1",
      title: "Gateway status",
      w: 1,
      x: 2,
      y: 0,
      widgetType: "gateway-status",
    },
    {
      h: 5,
      i: "sandbox-status#1",
      title: "Sandbox status",
      w: 1,
      x: 3,
      y: 0,
      widgetType: "sandbox-status",
    },
    {
      h: 4,
      i: "active-sandboxes#1",
      title: "Active sandboxes",
      w: 2,
      x: 0,
      y: 3.5,
      widgetType: "active-sandboxes",
    },
    {
      h: 4,
      i: "provisioning-snapshot#1",
      title: "Provisioning snapshot",
      w: 1,
      x: 2,
      y: 3.5,
      widgetType: "provisioning-snapshot",
    },
    {
      h: 4,
      i: "gateway-releases#1",
      title: "Gateway releases",
      w: 1,
      x: 3,
      y: 3.5,
      widgetType: "gateway-releases",
    },
    {
      h: 5,
      i: "attention-required#1",
      title: "Attention required",
      w: 2,
      x: 0,
      y: 7.5,
      widgetType: "attention-required",
    },
    {
      h: 5,
      i: "pod-and-sandbox-readiness#1",
      title: "Pod and sandbox readiness",
      w: 1,
      x: 2,
      y: 7.5,
      widgetType: "pod-and-sandbox-readiness",
    },
    {
      h: 5,
      i: "certificate-health#1",
      title: "Certificate health",
      w: 1,
      x: 3,
      y: 7.5,
      widgetType: "certificate-health",
    },
  ],
  lg: [],
  md: [],
  sm: [],
};
operationalLayout.lg = operationalLayout.xl;
operationalLayout.md = operationalLayout.xl;
operationalLayout.sm = operationalLayout.xl.map((item, index) => ({
  ...item,
  w: 1,
  x: 0,
  y: operationalLayout.xl
    .slice(0, index)
    .reduce((sum, previous) => sum + previous.h, 0),
}));

export function OperationalDashboardPageSurface() {
  return (
    <>
      <PageSection
        data-page-region="header"
        hasBodyWrapper={false}
        variant="default"
      >
        <Content>
          <Title headingLevel="h1" size="2xl">
            Operational overview
          </Title>
          <p>
            A compact landing page for the current state of the resources you
            are allowed to operate.
          </p>
        </Content>
      </PageSection>
      <PageSection
        data-page-region="body"
        hasBodyWrapper={false}
        isFilled
        variant="secondary"
      >
        <DashboardNavigation
          current="Overview"
          isAdmin={false}
          showLayoutActions={false}
          updatedLabel="Updated 2 minutes ago"
        />
        <DashboardPurpose>
          What needs attention right now? Are my gateways and sandboxes healthy?
          Did provisioning or capacity change enough to investigate?
        </DashboardPurpose>
        <DashboardFilters
          actions={
            <>
              <Button variant="link" onClick={() => undefined}>
                Reset to default layout
              </Button>
              <Button
                variant="secondary"
                icon={<PlusCircleIcon />}
                onClick={() => undefined}
              >
                Add widgets
              </Button>
            </>
          }
        />
        <MockupWidgetizedLayout template={operationalLayout}>
          <DashboardWidget
            title="Resource summary"
            icon={<ClusterIcon />}
            style={{ gridColumn: "1 / span 2", order: 1 }}
          >
            <DashboardSummaryList
              footnote="Counts respect your fleet and environment access."
              items={[
                {
                  label: "Gateways",
                  trend: { direction: "up", tone: "positive" },
                  value: "12",
                },
                {
                  label: "Sandboxes",
                  trend: { direction: "up", tone: "positive" },
                  value: "28",
                },
                { label: "Healthy resources", value: "35" },
                {
                  indicator: "attention",
                  label: "Resources needing attention",
                  value: "10",
                },
              ]}
            />
          </DashboardWidget>
          <DashboardWidget
            icon={<ContainerNodeIcon />}
            title="Gateway status"
            style={{ order: 2 }}
          >
            <DashboardDonut
              label="Gateways"
              value="12"
              segments={[
                { label: "Healthy", value: 9, color: "#63993d" },
                { label: "Provisioning", value: 1, color: "#0066cc" },
                { label: "Degraded", value: 1, color: "#f0ab00" },
                { label: "Failed", value: 1, color: "#c9190b" },
              ]}
            />
          </DashboardWidget>
          <DashboardWidget
            icon={<CubesIcon />}
            title="Sandbox status"
            style={{ order: 3 }}
          >
            <DashboardDonut
              label="Sandboxes"
              value="28"
              segments={[
                { label: "Active", value: 20, color: "#63993d" },
                { label: "Expiring", value: 4, color: "#f0ab00" },
                { label: "Idle", value: 3, color: "#0066cc" },
                { label: "Orphaned", value: 1, color: "#c9190b" },
              ]}
            />
          </DashboardWidget>
          <DashboardWidget
            title="Gateway releases"
            icon={<CodeBranchIcon />}
            style={{ gridColumn: "4", gridRow: "2", order: 6 }}
          >
            <DashboardSummaryList
              footnote="Gateway counts grouped by the resolved release version."
              gap={8}
              items={[
                { label: "OpenShell 2.1", value: "18" },
                { label: "OpenShell 2.0", value: "15" },
                { label: "OpenShell 1.9", value: "7" },
                { label: "Unknown release", value: "2" },
              ]}
            />
          </DashboardWidget>
          <DashboardWidget
            title="Active sandboxes"
            icon={<CubesIcon />}
            style={{ gridColumn: "1 / span 2", gridRow: "2", order: 4 }}
          >
            <DashboardSparkline
              caption="Last 7 days"
              data={[20, 24, 21, 27, 25, 30, 28]}
            />
          </DashboardWidget>
          <DashboardWidget
            title="Provisioning snapshot"
            icon={<CheckCircleIcon />}
            style={{ gridColumn: "3", gridRow: "2", order: 5 }}
          >
            <DashboardSummaryList
              footnote="Open Provisioning for failure reasons and time-to-ready trends."
              gap={8}
              items={[
                { label: "Success rate (24h)", value: "96.2%" },
                { label: "Median time to ready", value: "4m 48s" },
                { label: "In progress", value: "2 gateways" },
              ]}
            />
          </DashboardWidget>
          <DashboardWidget
            title="Attention required"
            icon={<ExclamationCircleIcon />}
            style={{ gridColumn: "1 / span 2", gridRow: "3", order: 7 }}
          >
            <DashboardSummaryList
              footnote="Counts are grouped by attention type. Open the resource lists for details."
              gap={8}
              items={[
                { label: "Failed gateways", value: "1" },
                { label: "Degraded gateways", value: "1" },
                { label: "Expiring sandboxes", value: "4" },
                { label: "Idle or orphaned sandboxes", value: "4" },
              ]}
            />
          </DashboardWidget>
          <DashboardWidget
            adminOnly
            title="Pod and sandbox readiness"
            icon={<CheckCircleIcon />}
            style={{ gridColumn: "3", gridRow: "3", order: 8 }}
          >
            <DashboardSummaryList
              gap={8}
              items={[
                { label: "Pod readiness", value: "96.4%" },
                { label: "Sandbox readiness", value: "92.9%" },
                {
                  indicator: "attention",
                  label: "Unready resources",
                  value: "3",
                },
              ]}
              footnote="Readiness is the percentage of resources whose Kubernetes Ready condition is true, not a count of Running or Pending pods."
            />
          </DashboardWidget>
          <DashboardWidget
            adminOnly
            title="Certificate health"
            icon={<ExclamationCircleIcon />}
            style={{ gridColumn: "4", gridRow: "3", order: 9 }}
          >
            <DashboardSummaryList
              gap={8}
              items={[
                { label: "Valid certificates", value: "9 / 10" },
                {
                  indicator: "attention",
                  label: "Days to expiry (soonest)",
                  value: "18 days",
                },
                { label: "Expired certificates", value: "0" },
                { label: "Status unavailable", value: "1" },
              ]}
              footnote="Certificate status is read from gateway and console TLS secrets; unreadable status shows Unavailable."
            />
          </DashboardWidget>
        </MockupWidgetizedLayout>
      </PageSection>
    </>
  );
}

export { OperationalDashboardMockup } from "./operational-dashboard-mockup";
