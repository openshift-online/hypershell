import {
  defaultOpenShellGatewayServiceAccountListRequest,
  GatewayPage,
  type OpenShellGatewayServiceAccountListRequest,
  type OpenShellGatewayServiceAccountSortField,
  type OpenShellGatewayServiceAccountStatus,
  toGatewayDetailTab,
} from "@openshift-online/hypershell-gateway-management-ui";
import { useParams, useSearchParams } from "react-router";
import { createPageMeta } from "../lib/page-meta";

export const meta = createPageMeta(
  "app.page.gatewayDetails.title",
  "app.page.gateway.description",
);

const serviceAccountStatuses = new Set<OpenShellGatewayServiceAccountStatus>([
  "deleting",
  "error",
  "expired",
  "provisioning",
  "ready",
  "revoked",
  "revoking",
]);
const serviceAccountSortFields =
  new Set<OpenShellGatewayServiceAccountSortField>([
    "created_at",
    "expires_at",
    "name",
    "role",
    "status",
  ]);

function positiveInteger(value: string | null, fallback: number): number {
  const parsed = Number(value);
  return Number.isInteger(parsed) && parsed > 0 ? parsed : fallback;
}

function serviceAccountCollectionState(
  searchParams: URLSearchParams,
): OpenShellGatewayServiceAccountListRequest {
  const defaults = defaultOpenShellGatewayServiceAccountListRequest;
  const status = searchParams.get("sa-status");
  const sort = searchParams.get("sa-sort");
  return {
    order: searchParams.get("sa-order") === "asc" ? "asc" : defaults.order,
    page: positiveInteger(searchParams.get("sa-page"), defaults.page),
    search: searchParams.get("sa-search") ?? defaults.search,
    size: Math.min(
      100,
      positiveInteger(searchParams.get("sa-size"), defaults.size),
    ),
    sort:
      sort &&
      serviceAccountSortFields.has(
        sort as OpenShellGatewayServiceAccountSortField,
      )
        ? (sort as OpenShellGatewayServiceAccountSortField)
        : defaults.sort,
    ...(status &&
    serviceAccountStatuses.has(status as OpenShellGatewayServiceAccountStatus)
      ? { status: status as OpenShellGatewayServiceAccountStatus }
      : {}),
  };
}

function setOrDelete(
  params: URLSearchParams,
  name: string,
  value: string,
  defaultValue: string,
) {
  if (value === defaultValue) {
    params.delete(name);
  } else {
    params.set(name, value);
  }
}

export default function GatewayRoute() {
  const { gatewayId = "" } = useParams();
  const [searchParams, setSearchParams] = useSearchParams();
  const activeTab = toGatewayDetailTab(searchParams.get("tab"));
  const collectionState = serviceAccountCollectionState(searchParams);

  return (
    <GatewayPage
      activeTab={activeTab}
      gatewayId={gatewayId}
      serviceAccountCollectionState={collectionState}
      onServiceAccountCollectionStateChange={(state, reason) => {
        setSearchParams(
          (previous) => {
            const next = new URLSearchParams(previous);
            const defaults = defaultOpenShellGatewayServiceAccountListRequest;
            setOrDelete(
              next,
              "sa-page",
              String(state.page),
              String(defaults.page),
            );
            setOrDelete(
              next,
              "sa-size",
              String(state.size),
              String(defaults.size),
            );
            setOrDelete(next, "sa-search", state.search, defaults.search);
            setOrDelete(next, "sa-sort", state.sort, defaults.sort);
            setOrDelete(next, "sa-order", state.order, defaults.order);
            setOrDelete(next, "sa-status", state.status ?? "", "");
            return next;
          },
          { replace: reason === "filter" },
        );
      }}
      onTabChange={(tab) => {
        setSearchParams((previous) => {
          const next = new URLSearchParams(previous);
          if (tab === "connection") {
            next.delete("tab");
          } else {
            next.set("tab", tab);
          }
          return next;
        });
      }}
    />
  );
}
