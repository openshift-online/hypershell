import { OperationalDashboardPage } from "@openshift-online/hypershell-operational-dashboard-ui";

import { RequireDashboardAdmin } from "../features/dashboard/require-dashboard-admin";
import { HomepagePage } from "../features/homepage/homepage";
import { isDashboardHost } from "../lib/dashboard-host";
import { createPageMeta } from "../lib/page-meta";

export const meta = createPageMeta(
  "app.page.homepage.title",
  "app.page.homepage.description",
);

export default function HomeRoute() {
  if (isDashboardHost(globalThis.location.hostname)) {
    return (
      <RequireDashboardAdmin>
        <OperationalDashboardPage />
      </RequireDashboardAdmin>
    );
  }
  return <HomepagePage />;
}
