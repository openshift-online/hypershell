import {
  GatewayPage,
  toGatewayDetailTab,
} from "@openshift-online/hypershell-gateway-management-ui";
import { useParams, useSearchParams } from "react-router";
import { createPageMeta } from "../lib/page-meta";

export const meta = createPageMeta(
  "app.page.gatewayDetails.title",
  "app.page.gateway.description",
);

export default function GatewayRoute() {
  const { gatewayId = "" } = useParams();
  const [searchParams, setSearchParams] = useSearchParams();
  const activeTab = toGatewayDetailTab(searchParams.get("tab"));

  return (
    <GatewayPage
      activeTab={activeTab}
      gatewayId={gatewayId}
      onTabChange={(tab) => {
        setSearchParams(
          (previous) => {
            const next = new URLSearchParams(previous);
            if (tab === "connection") {
              next.delete("tab");
            } else {
              next.set("tab", tab);
            }
            return next;
          },
          { replace: true },
        );
      }}
    />
  );
}
