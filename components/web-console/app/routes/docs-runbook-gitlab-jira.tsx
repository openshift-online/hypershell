import { RunbookPage } from "../features/docs/runbook-page";
import { createPageMeta } from "../lib/page-meta";

export const meta = createPageMeta(
  "app.docs.runbook.gitlab.title",
  "app.docs.runbook.gitlab.subtitle",
);

export default function DocsRunbookGitlabJiraRoute() {
  return <RunbookPage runbook="gitlab-jira" />;
}
