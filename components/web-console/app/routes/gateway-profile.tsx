import { GatewayProfilePage } from "@openshift-online/hypershell-gateway-management-ui";
import { useParams } from "react-router";

import { createPageMeta } from "../lib/page-meta";

export const meta = createPageMeta(
  "app.page.gatewayProfileDetails.title",
  "app.page.gatewayProfileDetails.description",
);

export default function GatewayProfileRoute() {
  const { gatewayProfileId = "" } = useParams();

  return <GatewayProfilePage gatewayProfileId={gatewayProfileId} />;
}
