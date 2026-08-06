import { GatewayCreatePage } from "../features/gateways/gateway-create";
import { createPageMeta } from "../lib/page-meta";

export const meta = createPageMeta(
  "app.page.gatewayProvision.title",
  "app.page.gatewayProvision.description",
);

export default GatewayCreatePage;
