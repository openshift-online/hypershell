import { GatewayCreatePage } from "@openshift-online/hypershell-gateway-ui";
import { createPageMeta } from "../lib/page-meta";

export const meta = createPageMeta(
  "app.page.gatewayProvision.title",
  "app.page.gatewayProvision.description",
);

export default GatewayCreatePage;
