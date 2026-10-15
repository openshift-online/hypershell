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
      "resource-summary",
      "gateway-status",
      "sandbox-status",
      "active-sandboxes",
      "provisioning-snapshot",
      "gateway-releases",
      "attention-required",
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
    const statusWidget = defaultDashboardLayoutTemplate.xl.find(
      (item) => item.widgetType === "gateway-status",
    );
    if (!statusWidget) {
      throw new Error("expected gateway status widget in default layout");
    }

    const duplicateStatus = {
      ...defaultDashboardLayoutTemplate,
      xl: [
        ...defaultDashboardLayoutTemplate.xl,
        {
          ...statusWidget,
          i: "gateway-status#2",
        },
      ],
    };

    const sanitized = sanitizeDashboardTemplate(duplicateStatus);

    expect(
      sanitized.xl.filter((item) => item.widgetType === "gateway-status"),
    ).toHaveLength(1);
  });

  it("keeps all overview widget types in a variant on save", () => {
    const sanitized = sanitizeDashboardTemplate(defaultDashboardLayoutTemplate);

    expect(new Set(sanitized.xl.map((item) => item.widgetType)).size).toBe(7);
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
    const reconciliationSuccessesWidget = {
      h: 5,
      i: "reconciliation-successes#1",
      title: "Reconciliation successes",
      w: 1,
      widgetType: "reconciliation-successes",
      x: 1,
      y: 14,
    };
    const reconciliationOverTimeWidget = {
      h: 7,
      i: "reconciliation-over-time#1",
      title: "Reconciliations over time",
      w: 3,
      widgetType: "reconciliation-over-time",
      x: 0,
      y: 19,
    };
    const templateWithRetiredWidget = {
      ...defaultDashboardLayoutTemplate,
      xl: [
        ...defaultDashboardLayoutTemplate.xl,
        sandboxesWidget,
        reconciliationSuccessesWidget,
        reconciliationOverTimeWidget,
      ],
    };

    const stripped = stripRemovedWidgetTypes(templateWithRetiredWidget);

    expect(
      stripped.xl.some((item) => item.widgetType === "provisioned-sandboxes"),
    ).toBe(false);
    expect(
      stripped.xl.some(
        (item) => item.widgetType === "reconciliation-successes",
      ),
    ).toBe(false);
    expect(
      stripped.xl.some(
        (item) => item.widgetType === "reconciliation-over-time",
      ),
    ).toBe(false);
  });
});
