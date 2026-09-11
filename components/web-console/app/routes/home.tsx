import { GatewaysPage } from "@openshift-online/hypershell-gateway-management-ui";
import { OperationalDashboardPage } from "@openshift-online/hypershell-operational-dashboard-ui";
import { useLocation, useNavigate, useSearchParams } from "react-router";

import { RequireDashboardAdmin } from "../features/dashboard/require-dashboard-admin";
import {
  parseGatewayListState,
  serializeGatewayListState,
} from "../features/gateways/gateway-list-state";
import { isDashboardHost } from "../lib/dashboard-host";
import { createPageMeta } from "../lib/page-meta";

export const meta = createPageMeta(
  "app.nav.gateways",
  "app.page.gateways.description",
);

export default function HomeRoute() {
  const location = useLocation();
  const navigate = useNavigate();
  const [searchParameters, setSearchParameters] = useSearchParams();
  if (isDashboardHost(globalThis.location.hostname)) {
    return (
      <RequireDashboardAdmin>
        <OperationalDashboardPage />
      </RequireDashboardAdmin>
    );
  }
  const collectionState = parseGatewayListState(searchParameters);
  const deletedGatewayName =
    typeof (location.state as { deletedGatewayName?: unknown } | null)
      ?.deletedGatewayName === "string"
      ? (location.state as { deletedGatewayName: string }).deletedGatewayName
      : undefined;

  return (
    <GatewaysPage
      collectionState={collectionState}
      deletedGatewayName={deletedGatewayName}
      onDismissDeletedGateway={() => {
        void navigate(`${location.pathname}${location.search}`, {
          replace: true,
          state: null,
        });
      }}
      onCollectionStateChange={(state, reason) => {
        setSearchParameters(serializeGatewayListState(state), {
          replace: reason === "filter",
        });
      }}
    />
  );
}
