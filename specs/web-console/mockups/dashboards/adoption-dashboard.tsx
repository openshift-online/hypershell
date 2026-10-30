import { Content, PageSection, Title } from "@patternfly/react-core";
import {
  ArrowRightIcon,
  ChartLineIcon,
  UsersIcon,
} from "@patternfly/react-icons";
import type { ExtendedTemplateConfig } from "@patternfly/widgetized-dashboard";
import {
  DashboardFilters,
  DashboardNavigation,
  DashboardPurpose,
  futureDashboards,
} from "../common/dashboard-controls";
import {
  DashboardDonut,
  DashboardSparkline,
  DashboardSummaryList,
  DashboardWidget,
  MockupWidgetizedLayout,
} from "../common/dashboard-widget";

const adoptionLayout: ExtendedTemplateConfig = {
  xl: [
    {
      h: 5,
      i: "adoption-summary#1",
      title: "Adoption summary",
      w: 2,
      x: 0,
      y: 0,
      widgetType: "adoption-summary",
    },
    {
      h: 5,
      i: "adoption-conversion#1",
      title: "Registration to first resource",
      w: 1,
      x: 2,
      y: 0,
      widgetType: "adoption-conversion",
    },
    {
      h: 6,
      i: "adoption-trends#1",
      title: "Adoption trends",
      w: 3,
      x: 0,
      y: 5,
      widgetType: "adoption-trends",
    },
  ],
  lg: [],
  md: [],
  sm: [],
};
adoptionLayout.lg = adoptionLayout.xl;
adoptionLayout.md = adoptionLayout.xl;
adoptionLayout.sm = adoptionLayout.xl.map((item, index) => ({
  ...item,
  w: 1,
  x: 0,
  y: adoptionLayout.xl
    .slice(0, index)
    .reduce((sum, previous) => sum + previous.h, 0),
}));

export function AdoptionDashboardPageSurface() {
  return (
    <>
      <PageSection
        data-page-region="header"
        hasBodyWrapper={false}
        variant="default"
      >
        <Content>
          <Title headingLevel="h1" size="2xl">
            Adoption
          </Title>
          <p>
            Understand who is using HyperShell, how adoption is changing, and
            where users drop off.
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
          current="Adoption"
          destinations={futureDashboards}
        />
        <DashboardPurpose>
          Are people adopting HyperShell? Are registrations, active use,
          conversion, and retention moving in the right direction? These are
          aggregate measures; no user-level data is displayed.
        </DashboardPurpose>
        <DashboardFilters />
        <MockupWidgetizedLayout
          columns={{ xl: 3, lg: 3, md: 3, sm: 1 }}
          template={adoptionLayout}
        >
          <DashboardWidget
            adminOnly
            icon={<UsersIcon />}
            title="Adoption summary"
          >
            <div style={{ display: "grid", gap: 16 }}>
              <Title headingLevel="h3" size="lg">
                450 registered users
              </Title>
              <DashboardSummaryList
                gap={8}
                items={[
                  {
                    label: "New registrations (selected period)",
                    value: "48",
                  },
                  { label: "Daily active users", value: "186" },
                  { label: "Weekly active users", value: "312" },
                  { label: "Monthly active users", value: "398" },
                  { label: "Inactive users (30+ days)", value: "52" },
                  { label: "7-day retention", value: "78.0" },
                  { label: "30-day retention", value: "-" },
                ]}
              />
            </div>
          </DashboardWidget>
          <DashboardWidget
            adminOnly
            icon={<ArrowRightIcon />}
            title="Registration to first resource"
          >
            <DashboardDonut
              label="Conversion"
              value="68%"
              segments={[
                {
                  label: "First gateway conversion",
                  value: 68,
                  color: "#0066cc",
                },
                { label: "Not converted", value: 32, color: "#d2d2d2" },
              ]}
            />
            <DashboardSummaryList
              gap={8}
              items={[
                { label: "First gateway conversion", value: "306" },
                { label: "First sandbox conversion", value: "-" },
                { label: "Median time to first resource", value: "-" },
              ]}
            />
          </DashboardWidget>
          <DashboardWidget
            adminOnly
            icon={<ChartLineIcon />}
            title="Adoption trends"
          >
            <div style={{ display: "grid", gap: 16 }}>
              <div style={{ display: "grid", gap: 8 }}>
                <Title headingLevel="h4" size="md">
                  New registrations
                </Title>
                <DashboardSparkline
                  caption="Last 30 days - daily bucket - UTC"
                  data={[18, 23, 27, 32, 31, 39, 44, 48, 55, 61, 67, 74]}
                />
              </div>
              <div style={{ display: "grid", gap: 8 }}>
                <Title headingLevel="h4" size="md">
                  Active users
                </Title>
                <DashboardSparkline
                  caption="Last 30 days - daily bucket - UTC"
                  data={[12, 15, 18, 24, 29, 33, 36, 41, 47, 52, 58, 64]}
                />
              </div>
            </div>
          </DashboardWidget>
        </MockupWidgetizedLayout>
      </PageSection>
    </>
  );
}
