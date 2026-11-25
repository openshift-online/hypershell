import { DocsHomePage } from "../features/docs/docs-home-page";
import { createPageMeta } from "../lib/page-meta";

export const meta = createPageMeta(
  "app.docs.home.title",
  "app.docs.home.description",
);

export default function DocsRoute() {
  return <DocsHomePage />;
}
