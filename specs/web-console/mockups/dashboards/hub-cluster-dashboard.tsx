import { Button, Content, PageSection, Title } from "@patternfly/react-core";
import type { ExtendedTemplateConfig } from "@patternfly/widgetized-dashboard";
import {
  ChartLineIcon,
  CubesIcon,
  HourglassHalfIcon,
  MemoryIcon,
  MicrochipIcon,
  PlusCircleIcon,
  ServerIcon,
  TachometerAltIcon,
} from "@patternfly/react-icons";
import { useState } from "react";
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

// Deliberately mirrors the production dashboard contract without importing
// production layout or widget code. Mockups remain standalone design artifacts.
const provisioningLayout: ExtendedTemplateConfig = {
  xl: [
    {
      h: 8,
      i: "system-summary#1",
      title: "System summary",
      w: 1,
      x: 0,
      y: 0,
      widgetType: "system-summary",
    },
    {
      h: 5,
      i: "memory#1",
      title: "Memory",
      w: 1,
      x: 1,
      y: 0,
      widgetType: "memory",
    },
    {
      h: 5,
      i: "cpu#1",
      title: "CPU",
      w: 1,
      x: 2,
      y: 0,
      widgetType: "cpu",
    },
    {
      h: 5,
      i: "nodes#1",
      title: "Nodes",
      w: 1,
      x: 3,
      y: 0,
      widgetType: "nodes",
    },
    {
      h: 5,
      i: "pods#1",
      title: "Pods",
      w: 1,
      x: 1,
      y: 5,
      widgetType: "pods",
    },
    {
      h: 5,
      i: "provision-time#1",
      title: "Provision time",
      w: 1,
      x: 2,
      y: 5,
      widgetType: "provision-time",
    },
    {
      h: 5,
      i: "provision-reliability#1",
      title: "Provision reliability",
      w: 1,
      x: 3,
      y: 5,
      widgetType: "provision-reliability",
    },
  ],
  lg: [],
  md: [],
  sm: [],
};
provisioningLayout.lg = provisioningLayout.xl;
provisioningLayout.md = provisioningLayout.xl;
let stackedY = 0;
provisioningLayout.sm = provisioningLayout.xl.map((item) => {
  const positioned = { ...item, h: 5, w: 1, x: 0, y: stackedY };
  stackedY += positioned.h;
  return positioned;
});

export function HubClusterDashboardPageSurface() {
  const [timeRange, setTimeRange] = useState<7 | 30>(7);
  const rangeLabel = `Last ${String(timeRange)} days`;
  const memoryTrend =
    timeRange === 7
      ? [52, 55, 54, 59, 61, 64, 68]
      : [
          43, 46, 48, 47, 51, 52, 54, 56, 55, 58, 59, 61, 60, 63, 64, 65, 64,
          66, 67, 68, 67, 69, 68, 70, 69, 68, 68, 67, 68, 68,
        ];
  const cpuTrend =
    timeRange === 7
      ? [48, 51, 49, 53, 50, 52, 54]
      : [
          42, 44, 43, 46, 45, 47, 48, 46, 49, 50, 48, 51, 50, 52, 51, 53, 52,
          54, 53, 55, 54, 53, 55, 54, 56, 55, 54, 53, 54, 54,
        ];
  const reliabilityTrend =
    timeRange === 7
      ? [92, 94, 93, 95, 96, 97, 96]
      : [
          89, 91, 90, 92, 91, 93, 94, 92, 93, 94, 95, 94, 96, 95, 96, 97, 95,
          96, 97, 96, 97, 98, 97, 96, 97, 96, 98, 97, 96, 96,
        ];

  return (
    <>
      <PageSection
        data-page-region="header"
        hasBodyWrapper={false}
        variant="default"
      >
        <Content>
          <Title headingLevel="h1" size="2xl">
            Provisioning dashboard
          </Title>
          <p>
            Monitor capacity, node health, and gateway provisioning across all
            managed clusters.
          </p>
        </Content>
      </PageSection>
      <PageSection data-page-region="body" isFilled variant="secondary">
        <DashboardNavigation current="Provisioning" showLayoutActions={false} />
        <DashboardPurpose>
          Is the managed-cluster fleet healthy enough to host HyperShell
          services and provision gateways? Review current utilization, node and
          pod health, and provisioning reliability.
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
          onTimeRangeChange={(range) => setTimeRange(range === 30 ? 30 : 7)}
          timeRange={timeRange}
          timeRanges={[7, 30]}
        />
        <MockupWidgetizedLayout template={provisioningLayout}>
          <DashboardWidget
            adminOnly
            title="System summary"
            icon={<TachometerAltIcon />}
          >
            <DashboardSummaryList
              footnote="System-wide managed-cluster metrics are visible to platform administrators."
              gap={18}
              items={[
                { label: "Managed clusters", value: "8" },
                { label: "Nodes", value: "12" },
                { label: "Pods", value: "184" },
                {
                  label: "Gateways",
                  trend: { direction: "up", tone: "positive" },
                  value: "42",
                },
                {
                  label: "Memory used",
                  trend: { direction: "up", tone: "negative" },
                  value: "68%",
                },
                {
                  label: "CPU used",
                  trend: { direction: "down", tone: "positive" },
                  value: "54%",
                },
              ]}
            />
          </DashboardWidget>
          <DashboardWidget adminOnly title="Memory" icon={<MemoryIcon />}>
            <DashboardDonut
              label="Memory"
              value="68%"
              segments={[
                { label: "Used", value: 68, color: "#0066cc" },
                { label: "Available", value: 32, color: "#d2d2d2" },
              ]}
            />
            <DashboardSparkline
              caption={`${rangeLabel} - daily used memory (GiB)`}
              data={memoryTrend}
            />
          </DashboardWidget>
          <DashboardWidget adminOnly title="CPU" icon={<MicrochipIcon />}>
            <DashboardDonut
              label="CPU"
              value="54%"
              segments={[
                { label: "Used", value: 54, color: "#6753ac" },
                { label: "Available", value: 46, color: "#d2d2d2" },
              ]}
            />
            <DashboardSparkline
              caption={`${rangeLabel} - daily used CPU (cores)`}
              data={cpuTrend}
            />
          </DashboardWidget>
          <DashboardWidget adminOnly title="Nodes" icon={<ServerIcon />}>
            <DashboardDonut
              label="Nodes"
              value="11 / 12"
              segments={[
                { label: "Ready", value: 11, color: "#3e8635" },
                { label: "Not ready", value: 1, color: "#c9190b" },
              ]}
            />
          </DashboardWidget>
          <DashboardWidget adminOnly title="Pods" icon={<CubesIcon />}>
            <DashboardDonut
              label="Pods"
              value="184"
              segments={[
                { label: "Running", value: 164, color: "#3e8635" },
                { label: "Pending", value: 8, color: "#f0ab00" },
                { label: "Failed", value: 4, color: "#c9190b" },
                { label: "Succeeded", value: 8, color: "#0066cc" },
                { label: "Unknown", value: 1, color: "#8a8d90" },
                { label: "Unused", value: 15, color: "#d2d2d2" },
              ]}
            />
          </DashboardWidget>
          <DashboardWidget
            adminOnly
            title="Provision time"
            icon={<HourglassHalfIcon />}
          >
            <DashboardSummaryList
              gap={8}
              items={[
                { label: "Average", value: "5m 12s" },
                { label: "P50", value: "4m 48s" },
                { label: "P95", value: "12m 06s" },
              ]}
            />
          </DashboardWidget>
          <DashboardWidget
            adminOnly
            title="Provision reliability"
            icon={<ChartLineIcon />}
          >
            <DashboardSummaryList
              gap={8}
              items={[
                { label: "Success rate (24h)", value: "96.2%" },
                { label: "Successful", value: "52" },
                { label: "Failed", value: "2" },
                {
                  label: "Success rate change",
                  trend: { direction: "up", tone: "positive" },
                  value: "+3%",
                },
              ]}
            >
              <DashboardSparkline
                caption={`${rangeLabel} - daily success rate`}
                data={reliabilityTrend}
              />
            </DashboardSummaryList>
          </DashboardWidget>
        </MockupWidgetizedLayout>
      </PageSection>
    </>
  );
}
