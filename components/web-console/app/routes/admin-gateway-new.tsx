import { AdminGatewayCreatePage } from "../features/gateways/admin-gateway-create";
import { createPageMeta } from "../lib/page-meta";

export const meta = createPageMeta(
  "app.page.gatewayProvision.title",
  "app.page.gatewayProvision.description",
  "app.adminProductName",
);

export default AdminGatewayCreatePage;
