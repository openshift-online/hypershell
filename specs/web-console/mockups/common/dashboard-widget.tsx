import {
  ChartArea,
  ChartDonut,
  ChartDonutUtilization,
  ChartGroup,
  ChartLine,
  ChartThemeColor,
} from "@patternfly/react-charts/victory";
import {
  Card,
  CardBody,
  Content,
  Divider,
  Label,
} from "@patternfly/react-core";
import {
  EllipsisVIcon,
  ExclamationCircleIcon,
  GripVerticalIcon,
  TrendDownIcon,
  TrendUpIcon,
} from "@patternfly/react-icons";
import {
  GridLayout,
  WidgetDrawer,
  type ExtendedTemplateConfig,
  type Variants,
  type WidgetMapping,
} from "@patternfly/widgetized-dashboard";
import "@patternfly/widgetized-dashboard/dist/esm/styles.css";
import "./dashboard-widget.css";
import {
  Children,
  cloneElement,
  isValidElement,
  useMemo,
  useState,
  useEffect,
  useRef,
} from "react";

export type DashboardSummaryTone = "positive" | "negative";

export type DashboardSummaryTrend = Readonly<{
  direction: "up" | "down";
  tone: DashboardSummaryTone;
}>;

export type DashboardSummaryItem = Readonly<{
  indicator?: "attention";
  label: string;
  trend?: DashboardSummaryTrend;
  value: string;
}>;

function DashboardSummaryRow({ item }: { item: DashboardSummaryItem }) {
  const TrendIcon =
    item.trend?.direction === "down" ? TrendDownIcon : TrendUpIcon;
  const indicatorColor =
    item.trend?.tone === "negative" || item.indicator === "attention"
      ? "var(--pf-t--global--icon--color--status--danger--default)"
      : "var(--pf-t--global--icon--color--status--success--default)";

  return (
    <div
      style={{
        alignItems: "center",
        display: "flex",
        fontSize: 12,
        justifyContent: "space-between",
      }}
    >
      <span>{item.label}</span>
      <strong style={{ alignItems: "center", display: "flex", gap: 4 }}>
        <span
          style={{
            display: "inline-flex",
            justifyContent: "center",
            width: 20,
          }}
        >
          {item.trend || item.indicator ? (
            <span style={{ color: indicatorColor }}>
              {item.indicator === "attention" ? (
                <ExclamationCircleIcon />
              ) : item.trend ? (
                <TrendIcon />
              ) : null}
            </span>
          ) : null}
        </span>
        <span style={{ textAlign: "right", whiteSpace: "nowrap" }}>
          {item.value}
        </span>
      </strong>
    </div>
  );
}

export function DashboardSummaryList({
  children,
  footnote,
  gap = 16,
  items,
}: {
  children?: React.ReactNode;
  footnote?: React.ReactNode;
  gap?: number;
  items: readonly DashboardSummaryItem[];
}) {
  return (
    <div style={{ display: "grid", gap }}>
      {items.map((item) => (
        <DashboardSummaryRow item={item} key={item.label} />
      ))}
      {children}
      {footnote ? <Content component="small">{footnote}</Content> : null}
    </div>
  );
}

export function MockupWidgetizedLayout({
  template,
  widgetMapping,
  children,
  columns = { xl: 4, lg: 4, md: 4, sm: 1 },
}: {
  template?: ExtendedTemplateConfig;
  widgetMapping?: WidgetMapping;
  children?: React.ReactNode;
  columns?: Record<Variants, number>;
}) {
  const childWidgets = Children.toArray(children);
  const resolvedTemplate =
    template ??
    ({
      xl: childWidgets.map((_, index) => ({
        h: 4,
        i: `mockup-widget-${index}`,
        title: `Mockup widget ${index + 1}`,
        w: 1,
        x: index % 4,
        y: Math.floor(index / 4) * 4,
        widgetType: `mockup-widget-${index}`,
      })),
      lg: childWidgets.map((_, index) => ({
        h: 4,
        i: `mockup-widget-${index}`,
        title: `Mockup widget ${index + 1}`,
        w: 1,
        x: index % 4,
        y: Math.floor(index / 4) * 4,
        widgetType: `mockup-widget-${index}`,
      })),
      md: childWidgets.map((_, index) => ({
        h: 4,
        i: `mockup-widget-${index}`,
        title: `Mockup widget ${index + 1}`,
        w: 1,
        x: index % 4,
        y: Math.floor(index / 4) * 4,
        widgetType: `mockup-widget-${index}`,
      })),
      sm: childWidgets.map((_, index) => ({
        h: 4,
        i: `mockup-widget-${index}`,
        title: `Mockup widget ${index + 1}`,
        w: 1,
        x: 0,
        y: index * 4,
        widgetType: `mockup-widget-${index}`,
      })),
    } satisfies ExtendedTemplateConfig);
  const resolvedMapping =
    widgetMapping ??
    (Object.fromEntries(
      childWidgets.map((child, index) => {
        const widgetType =
          template?.xl[index]?.widgetType ?? `mockup-widget-${index}`;
        const widgetProps = isValidElement<{
          adminOnly?: boolean;
          title?: string;
          icon?: React.ReactNode;
        }>(child)
          ? child.props
          : undefined;
        const title = widgetProps?.title ?? `Mockup widget ${index + 1}`;
        const content = isValidElement<{ showHeader?: boolean }>(child)
          ? cloneElement(child, { showHeader: false })
          : child;
        return [
          widgetType,
          {
            config: {
              icon: widgetProps?.icon,
              title,
              ...(widgetProps?.adminOnly
                ? { wrapperProps: { className: "mockup-widget-system" } }
                : {}),
            },
            defaults: { h: 4, maxH: 6, minH: 2, w: 1 },
            renderWidget: () => content,
          },
        ];
      }),
    ) as unknown as WidgetMapping);
  const [drawerOpen, setDrawerOpen] = useState(false);
  const [droppingWidgetType, setDroppingWidgetType] = useState<string>();
  const activeWidgetTypes = useMemo(
    () => [
      ...new Set(
        (
          Object.values(
            resolvedTemplate,
          ) as ExtendedTemplateConfig[keyof ExtendedTemplateConfig][]
        )
          .flat()
          .map((item) => item.widgetType),
      ),
    ],
    [resolvedTemplate],
  );

  return (
    <WidgetDrawer
      currentlyUsedWidgets={activeWidgetTypes}
      isOpen={drawerOpen}
      onOpenChange={setDrawerOpen}
      onWidgetDragEnd={() => setDroppingWidgetType(undefined)}
      onWidgetDragStart={setDroppingWidgetType}
      widgetMapping={resolvedMapping}
    >
      <GridLayout
        columns={columns}
        droppingWidgetType={droppingWidgetType}
        onDrawerExpandChange={setDrawerOpen}
        resizeWidgetConfig={{}}
        template={resolvedTemplate}
        widgetMapping={resolvedMapping}
      />
    </WidgetDrawer>
  );
}

export function DashboardWidget({
  title,
  icon,
  children,
  style,
  adminOnly = false,
  showHeader = true,
}: {
  title: string;
  icon?: React.ReactNode;
  children: React.ReactNode;
  style?: React.CSSProperties;
  adminOnly?: boolean;
  showHeader?: boolean;
}) {
  return (
    <Card
      isFullHeight
      style={{
        border: showHeader ? undefined : "0",
        borderRadius: showHeader ? undefined : "0",
        boxShadow: showHeader ? undefined : "none",
        overflow: "hidden",
        ...style,
      }}
    >
      {showHeader ? (
        <div
          style={{
            alignItems: "center",
            background: "#f0f0f0",
            borderBottom: "1px solid #d2d2d2",
            boxSizing: "border-box",
            display: "flex",
            flex: "0 0 40px",
            height: 40,
            justifyContent: "space-between",
            minHeight: 40,
            padding: "0 14px",
          }}
        >
          <span style={{ alignItems: "center", display: "flex", gap: 8 }}>
            {icon}
            <Content
              style={{
                fontSize: 13,
                fontWeight: 600,
                lineHeight: "40px",
                margin: 0,
              }}
            >
              {title}
            </Content>
            {adminOnly ? (
              <Label isCompact color="orange">
                System
              </Label>
            ) : null}
          </span>
          <span style={{ display: "flex", gap: 14 }}>
            <EllipsisVIcon />
            <GripVerticalIcon />
          </span>
        </div>
      ) : null}
      <CardBody style={{ boxSizing: "border-box", padding: 16 }}>
        {children}
      </CardBody>
    </Card>
  );
}
export function DashboardSparkline({
  caption = "Last 7 days",
  data = [22, 28, 32, 36, 42, 48, 51, 58, 64, 70, 76, 84],
}: {
  caption?: string;
  data?: number[];
}) {
  const containerRef = useRef<HTMLDivElement>(null);
  const [width, setWidth] = useState(220);

  useEffect(() => {
    const node = containerRef.current;
    if (!node) {
      return;
    }

    const observer = new ResizeObserver((entries) => {
      const nextWidth = entries[0]?.contentRect.width;
      if (nextWidth && nextWidth > 0) {
        setWidth(nextWidth);
      }
    });
    observer.observe(node);

    return () => {
      observer.disconnect();
    };
  }, []);

  return (
    <div ref={containerRef} style={{ width: "100%" }}>
      <ChartGroup
        ariaTitle="Trend"
        height={56}
        padding={{ bottom: 1, left: 2, right: 2, top: 1 }}
        themeColor={ChartThemeColor.blue}
        width={width}
      >
        <ChartArea data={data.map((y, x) => ({ x, y }))} />
      </ChartGroup>
      <small style={{ display: "block", textAlign: "center" }}>{caption}</small>
    </div>
  );
}

export function DashboardTrendChart({
  caption,
  series,
}: {
  caption: string;
  series: Array<{
    color: string;
    data: Array<{ x: number; y?: number }>;
    name: string;
  }>;
}) {
  return (
    <div style={{ width: "100%" }}>
      <ChartGroup
        ariaTitle={caption}
        height={110}
        padding={{ bottom: 10, left: 30, right: 10, top: 10 }}
        width={300}
      >
        {series.map((item) => (
          <ChartLine
            data={item.data}
            key={item.name}
            name={item.name}
            style={{ data: { stroke: item.color, strokeWidth: 2 } }}
          />
        ))}
      </ChartGroup>
      <div style={{ display: "flex", gap: 12, justifyContent: "center" }}>
        {series.map((item) => (
          <small key={item.name}>
            <span style={{ color: item.color }}>●</span> {item.name}
          </small>
        ))}
      </div>
      <small style={{ display: "block", textAlign: "center" }}>{caption}</small>
    </div>
  );
}
export function DashboardDonut({
  value,
  label,
  color = "#3e8635",
  segments,
}: {
  value: string;
  label: string;
  color?: string;
  segments?: Array<{ label: string; value: number; color: string }>;
}) {
  const data = segments ?? [{ label, value: 75, color }];
  return (
    <div
      style={{
        alignItems: "center",
        display: "flex",
        justifyContent: "center",
        padding: "8px 0 12px",
      }}
    >
      <ChartDonut
        ariaTitle={label}
        colorScale={data.map((segment) => segment.color)}
        data={data.map((segment) => ({ x: segment.label, y: segment.value }))}
        height={130}
        labels={({ datum }) => `${datum.x}: ${datum.y}`}
        legendData={data.map((segment) => ({
          name: `${segment.label}: ${segment.value}`,
        }))}
        legendOrientation="vertical"
        legendPosition="right"
        padding={{ bottom: 10, left: 10, right: 100, top: 10 }}
        subTitle={label}
        subTitlePosition="bottom"
        title={value}
        width={300}
      />
    </div>
  );
}
export function DashboardUtilization({
  value,
  total,
  unit,
  color = "#b6380b",
}: {
  value: number;
  total: number;
  unit: string;
  color?: string;
}) {
  return (
    <div
      style={{
        alignItems: "center",
        display: "flex",
        gap: 14,
        justifyContent: "center",
        padding: "14px 0",
      }}
    >
      <ChartDonutUtilization
        ariaTitle={`${value} ${unit} of ${total} ${unit}`}
        data={{ x: unit, y: Math.round((value / total) * 100) }}
        height={130}
        labels={() => `${Math.round((value / total) * 100)}%`}
        padding={{ bottom: 10, left: 16, right: 10, top: 10 }}
        thresholds={[{ value: 60 }, { value: 90 }]}
        width={130}
      />
      <div>
        <strong style={{ display: "block", fontSize: 16 }}>
          {value} {unit}
        </strong>
        <small>
          of {total} {unit}
        </small>
      </div>
    </div>
  );
}
export function WidgetDivider() {
  return <Divider />;
}
