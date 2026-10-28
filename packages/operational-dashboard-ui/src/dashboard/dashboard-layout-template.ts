import type { ExtendedTemplateConfig } from "@patternfly/widgetized-dashboard";
import type { IntlShape, MessageDescriptor } from "react-intl";

import { messages } from "../messages";

export const DASHBOARD_COLUMN_COUNT = 4;
export const RESOURCE_SUMMARY_WIDGET_HEIGHT = 5;
export const STATUS_WIDGET_HEIGHT = 3.5;
export const GATEWAY_STATUS_WIDGET_HEIGHT = 5;
export const ACTIVE_SANDBOXES_WIDGET_HEIGHT = 4;
export const PROVISIONING_SNAPSHOT_WIDGET_HEIGHT = 4;
export const GATEWAY_RELEASES_WIDGET_HEIGHT = 4;
export const ADOPTION_GATEWAY_RELEASES_WIDGET_HEIGHT =
  GATEWAY_RELEASES_WIDGET_HEIGHT;
export const ATTENTION_REQUIRED_WIDGET_HEIGHT = RESOURCE_SUMMARY_WIDGET_HEIGHT;
export const READINESS_WIDGET_HEIGHT = RESOURCE_SUMMARY_WIDGET_HEIGHT;
export const CERTIFICATE_HEALTH_WIDGET_HEIGHT = RESOURCE_SUMMARY_WIDGET_HEIGHT;

// Compatibility exports used by the sibling reliability dashboard and tests.
export const TITLE_WIDGET_HEIGHT = 1;
export const SECTION_TITLE_WIDGET_TYPE = "section-title";
export const USAGE_SUMMARY_WIDGET_HEIGHT = RESOURCE_SUMMARY_WIDGET_HEIGHT;
export const SYSTEM_SUMMARY_WIDGET_HEIGHT = 13;
export const INVENTORY_SUMMARY_WIDGET_HEIGHT = 6;
export const NODE_STATUS_WIDGET_HEIGHT = 4;
export const POD_CAPACITY_WIDGET_HEIGHT = 6;
export const PROVISION_RELIABILITY_WIDGET_HEIGHT = 8;
export const PROVISION_TIME_WIDGET_HEIGHT = 4;
export const REGISTERED_USERS_WIDGET_HEIGHT = 7;
export const UTILIZATION_WIDGET_HEIGHT = 6;
export const ADOPTION_GATEWAY_STATUS_WIDGET_HEIGHT = STATUS_WIDGET_HEIGHT;
export const ADOPTION_SANDBOX_STATUS_WIDGET_HEIGHT = STATUS_WIDGET_HEIGHT;

const WIDGET_TITLE_MESSAGES: Record<string, MessageDescriptor> = {
  "active-sandboxes": messages.activeSandboxesWidget,
  "attention-required": messages.attentionRequiredWidget,
  "gateway-releases": messages.gatewayReleasesWidget,
  "gateway-status": messages.gatewayStatusWidget,
  "provisioning-snapshot": messages.provisioningSnapshotWidget,
  "resource-summary": messages.resourceSummaryWidget,
  "sandbox-status": messages.sandboxStatusWidget,
  "pod-and-sandbox-readiness": messages.podAndSandboxReadinessWidget,
  "certificate-health": messages.certificateHealthWidget,
};

const desktopLayout = [
  {
    h: RESOURCE_SUMMARY_WIDGET_HEIGHT,
    i: "resource-summary#1",
    title: "Resource summary",
    w: 2,
    widgetType: "resource-summary",
    x: 0,
    y: 0,
  },
  {
    h: GATEWAY_STATUS_WIDGET_HEIGHT,
    i: "gateway-status#1",
    title: "Gateway status",
    w: 1,
    widgetType: "gateway-status",
    x: 2,
    y: 0,
  },
  {
    h: GATEWAY_STATUS_WIDGET_HEIGHT,
    i: "sandbox-status#1",
    title: "Sandbox status",
    w: 1,
    widgetType: "sandbox-status",
    x: 3,
    y: 0,
  },
  {
    h: ACTIVE_SANDBOXES_WIDGET_HEIGHT,
    i: "active-sandboxes#1",
    title: "Active sandboxes",
    w: 2,
    widgetType: "active-sandboxes",
    x: 0,
    y: 3.5,
  },
  {
    h: PROVISIONING_SNAPSHOT_WIDGET_HEIGHT,
    i: "provisioning-snapshot#1",
    title: "Provisioning snapshot",
    w: 1,
    widgetType: "provisioning-snapshot",
    x: 2,
    y: 3.5,
  },
  {
    h: GATEWAY_RELEASES_WIDGET_HEIGHT,
    i: "gateway-releases#1",
    title: "Gateway releases",
    w: 1,
    widgetType: "gateway-releases",
    x: 3,
    y: 3.5,
  },
  {
    h: ATTENTION_REQUIRED_WIDGET_HEIGHT,
    i: "attention-required#1",
    title: "Attention required",
    w: 2,
    widgetType: "attention-required",
    x: 0,
    y: 7.5,
  },
  {
    h: READINESS_WIDGET_HEIGHT,
    i: "pod-and-sandbox-readiness#1",
    title: "Pod and sandbox readiness",
    w: 1,
    widgetType: "pod-and-sandbox-readiness",
    x: 2,
    y: 7.5,
  },
  {
    h: CERTIFICATE_HEALTH_WIDGET_HEIGHT,
    i: "certificate-health#1",
    title: "Certificate health",
    w: 1,
    widgetType: "certificate-health",
    x: 3,
    y: 7.5,
  },
] as const;

const mobileLayout = desktopLayout.map((item, index) => ({
  ...item,
  w: 1,
  x: 0,
  y: desktopLayout
    .slice(0, index)
    .reduce((sum, previous) => sum + previous.h, 0),
}));

export const defaultDashboardLayoutTemplate: ExtendedTemplateConfig = {
  xl: [...desktopLayout],
  lg: [...desktopLayout],
  md: [...desktopLayout],
  sm: [...mobileLayout],
};

export function localizeDashboardLayoutTemplate(
  template: ExtendedTemplateConfig,
  intl: IntlShape,
): ExtendedTemplateConfig {
  return (Object.keys(template) as (keyof ExtendedTemplateConfig)[]).reduce(
    (localized, variant) => {
      localized[variant] = template[variant].map((item) => {
        const message = WIDGET_TITLE_MESSAGES[item.widgetType];
        return {
          ...item,
          title: message ? intl.formatMessage(message) : item.title,
        };
      });
      return localized;
    },
    {} as ExtendedTemplateConfig,
  );
}
