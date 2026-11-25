import { InstallHsctlPage } from "../features/docs/install-hsctl-page";
import { createPageMeta } from "../lib/page-meta";

export const meta = createPageMeta(
  "app.docs.install.title",
  "app.docs.install.subtitle",
);

export default function DocsInstallHsctlRoute() {
  return <InstallHsctlPage />;
}
