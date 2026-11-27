import { RunbookPage } from "../features/docs/runbook-page";
import { createPageMeta } from "../lib/page-meta";

export const meta = createPageMeta(
  "app.docs.runbook.github.title",
  "app.docs.runbook.github.subtitle",
);

export default function DocsRunbookGithubJiraRoute() {
  return <RunbookPage runbook="github-jira" />;
}
