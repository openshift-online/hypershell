import { GatewaysPage } from "@openshift-online/hypershell-gateway-ui";
import { useLocation, useNavigate } from "react-router";
import { createPageMeta } from "../lib/page-meta";

export const meta = createPageMeta(
  "app.nav.gateways",
  "app.page.gateways.description",
);

export default function HomeRoute() {
  const location = useLocation();
  const navigate = useNavigate();
  const deletedGatewayName =
    typeof (location.state as { deletedGatewayName?: unknown } | null)
      ?.deletedGatewayName === "string"
      ? (location.state as { deletedGatewayName: string }).deletedGatewayName
      : undefined;

  return (
    <GatewaysPage
      deletedGatewayName={deletedGatewayName}
      onDismissDeletedGateway={() => {
        void navigate(location.pathname, { replace: true, state: null });
      }}
    />
  );
}
