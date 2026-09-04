import { describe, expect, it } from "vitest";
import type { ExtendedTemplateConfig } from "@patternfly/widgetized-dashboard";

import { defaultDashboardLayoutTemplate } from "./dashboard-layout-template";
import {
  getActiveWidgetTypes,
  isValidSavedTemplate,
  sanitizeDashboardTemplate,
} from "./dashboard-layout-persistence";

describe("dashboard layout persistence", () => {
  it("collects active widget types across responsive variants", () => {
    expect(getActiveWidgetTypes(defaultDashboardLayoutTemplate)).toEqual([
      "usage-summary",
      "gateway-status",
      "provision-time",
      "provisioned-sandboxes",
      "memory",
      "system-summary",
      "registered-users",
      "cpu",
      "pods",
      "nodes",
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
});
