import { ResourceLimitsPage } from "../features/docs/resource-limits-page";
import { createPageMeta } from "../lib/page-meta";

export const meta = createPageMeta(
  "app.docs.refusal.title",
  "app.docs.refusal.statement",
);

export default function DocsResourceLimitsRoute() {
  return <ResourceLimitsPage />;
}
