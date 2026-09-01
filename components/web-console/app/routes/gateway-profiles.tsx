import { GatewayProfilesPage } from "@openshift-online/hypershell-gateway-management-ui";
import { useLocation, useNavigate, useSearchParams } from "react-router";

import {
  parseGatewayProfileListState,
  serializeGatewayProfileListState,
} from "../features/gateway-profiles/gateway-profile-list-state";
import { createPageMeta } from "../lib/page-meta";

export const meta = createPageMeta(
  "app.nav.gatewayProfiles",
  "app.page.gatewayProfiles.description",
);

export default function GatewayProfilesRoute() {
  const location = useLocation();
  const navigate = useNavigate();
  const [searchParameters, setSearchParameters] = useSearchParams();
  const collectionState = parseGatewayProfileListState(searchParameters);
  const deletedGatewayProfileName =
    typeof (location.state as { deletedGatewayProfileName?: unknown } | null)
      ?.deletedGatewayProfileName === "string"
      ? (location.state as { deletedGatewayProfileName: string })
          .deletedGatewayProfileName
      : undefined;

  return (
    <GatewayProfilesPage
      collectionState={collectionState}
      deletedGatewayProfileName={deletedGatewayProfileName}
      onDismissDeletedGatewayProfile={() => {
        void navigate(`${location.pathname}${location.search}`, {
          replace: true,
          state: null,
        });
      }}
      onCollectionStateChange={(state, reason) => {
        setSearchParameters(serializeGatewayProfileListState(state), {
          replace: reason === "filter",
        });
      }}
    />
  );
}
