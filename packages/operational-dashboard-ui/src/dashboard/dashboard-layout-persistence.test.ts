import { describe, expect, it } from "vitest";
import type { ExtendedTemplateConfig } from "@patternfly/widgetized-dashboard";

import { defaultDashboardLayoutTemplate } from "./dashboard-layout-template";
import {
  getActiveWidgetTypes,
  isValidSavedTemplate,
  sanitizeDashboardTemplate,
  stripRemovedWidgetTypes,
} from "./dashboard-layout-persistence";

describe("dashboard layout persistence", () => {
  it("collects active widget types across responsive variants", () => {
    expect(getActiveWidgetTypes(defaultDashboardLayoutTemplate)).toEqual([
      "section-title",
      "usage-summary",
      "registered-users",
      "gateway-status",
      "sandbox-status",
      "gateway-releases",
      "system-summary",
      "memory",
      "cpu",
      "nodes",
      "pods",
      "provision-time",
      "provision-reliability",
      "inventory-summary",
      "managed-cluster-providers",
      "managed-cluster-regions",
      "managed-database-status",
    ]);
  });

  it("accepts saved templates that define every responsive variant", () => {
    expect(
      isValidSavedTemplate(
        defaultDashboardLayoutTemplate,
        defaultDashboardLayoutTemplate,
      ),
    ).toBe(true);
  });

  it("rejects saved templates that omit a responsive variant", () => {
    expect(
      isValidSavedTemplate(
        { xl: defaultDashboardLayoutTemplate.xl } as ExtendedTemplateConfig,
        defaultDashboardLayoutTemplate,
      ),
    ).toBe(false);
  });

  it("deduplicates widget types within a variant on save", () => {
    const cpuWidget = defaultDashboardLayoutTemplate.xl.find(
      (item) => item.widgetType === "cpu",
    );
    if (!cpuWidget) {
      throw new Error("expected cpu widget in default layout");
    }

    const duplicateCpu = {
      ...defaultDashboardLayoutTemplate,
      xl: [
        ...defaultDashboardLayoutTemplate.xl,
        {
          ...cpuWidget,
          i: "cpu#2",
        },
      ],
    };

    const sanitized = sanitizeDashboardTemplate(duplicateCpu);

    expect(
      sanitized.xl.filter((item) => item.widgetType === "cpu"),
    ).toHaveLength(1);
  });

  it("keeps multiple section title widgets in a variant on save", () => {
    const sanitized = sanitizeDashboardTemplate(defaultDashboardLayoutTemplate);

    expect(
      sanitized.xl.filter((item) => item.widgetType === "section-title"),
    ).toHaveLength(3);
  });

  it("removes retired widget types from saved templates", () => {
    const sandboxesWidget = {
      h: 3,
      i: "provisioned-sandboxes#1",
      title: "Sandboxes",
      w: 1,
      widgetType: "provisioned-sandboxes",
      x: 3,
      y: 0,
    };
    const templateWithRetiredWidget = {
      ...defaultDashboardLayoutTemplate,
      xl: [...defaultDashboardLayoutTemplate.xl, sandboxesWidget],
    };

    const stripped = stripRemovedWidgetTypes(templateWithRetiredWidget);

    expect(
      stripped.xl.some((item) => item.widgetType === "provisioned-sandboxes"),
    ).toBe(false);
  });
});
