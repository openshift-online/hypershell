import type {
  ExtendedTemplateConfig,
  Variants,
} from "@patternfly/widgetized-dashboard";

/** Widget types removed from the catalog; stripped from saved layouts on load. */
const REMOVED_WIDGET_TYPES = new Set([
  "managed-clusters",
  "managed-cluster-status",
  "managed-databases",
]);

export function stripRemovedWidgetTypes(
  template: ExtendedTemplateConfig,
): ExtendedTemplateConfig {
  return (Object.keys(template) as Variants[]).reduce((acc, variant) => {
    acc[variant] = template[variant].filter(
      (item) => !REMOVED_WIDGET_TYPES.has(item.widgetType),
    );
    return acc;
  }, {} as ExtendedTemplateConfig);
}

export function getActiveWidgetTypes(
  template: ExtendedTemplateConfig,
): string[] {
  const types = new Set<string>();

  for (const variant of Object.keys(template) as Variants[]) {
    for (const item of template[variant]) {
      types.add(item.widgetType);
    }
  }

  return [...types];
}

export function isValidSavedTemplate(
  savedTemplate: ExtendedTemplateConfig,
  baseTemplate: ExtendedTemplateConfig,
): boolean {
  return (Object.keys(baseTemplate) as Variants[]).every((variant) =>
    Array.isArray(savedTemplate[variant]),
  );
}

export function sanitizeDashboardTemplate(
  template: ExtendedTemplateConfig,
): ExtendedTemplateConfig {
  return (Object.keys(template) as Variants[]).reduce((acc, variant) => {
    const seenIds = new Set<string>();
    const seenMetricWidgetTypes = new Set<string>();

    acc[variant] = template[variant].filter((item) => {
      if (seenIds.has(item.i)) {
        return false;
      }
      seenIds.add(item.i);

      if (item.widgetType === "section-title") {
        return true;
      }

      if (seenMetricWidgetTypes.has(item.widgetType)) {
        return false;
      }

      seenMetricWidgetTypes.add(item.widgetType);
      return true;
    });
    return acc;
  }, {} as ExtendedTemplateConfig);
}
