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
      "provisioned-sandboxes",
      "gateway-status",
      "system-summary",
      "memory",
      "provision-time",
      "cpu",
      "pods",
      "nodes",
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

  it("strips retired widget types from saved layouts", () => {
    const retiredWidget = {
      h: 3,
      i: "managed-clusters#1",
      title: "Clusters",
      w: 1,
      widgetType: "managed-clusters",
      x: 0,
      y: 99,
    };
    const withRetired = {
      ...defaultDashboardLayoutTemplate,
      xl: [...defaultDashboardLayoutTemplate.xl, retiredWidget],
    };

    const stripped = stripRemovedWidgetTypes(withRetired);

    expect(
      stripped.xl.filter((item) => item.widgetType === "managed-clusters"),
    ).toHaveLength(0);
    expect(stripped.xl).toHaveLength(defaultDashboardLayoutTemplate.xl.length);
  });
});
