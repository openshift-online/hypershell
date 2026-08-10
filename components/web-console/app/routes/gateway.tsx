import { GatewayPage } from "@openshift-online/hypershell-gateway-management-ui";
import { useParams } from "react-router";
import { createPageMeta } from "../lib/page-meta";

export const meta = createPageMeta(
  "app.page.gatewayDetails.title",
  "app.page.gateway.description",
);

export default function GatewayRoute() {
  const { gatewayId = "" } = useParams();

  return <GatewayPage gatewayId={gatewayId} />;
}
