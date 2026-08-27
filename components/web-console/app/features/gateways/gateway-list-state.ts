import {
  defaultGatewayListRequest,
  gatewayListPageSizes,
  type GatewayListRequest,
} from "@openshift-online/hypershell-gateway-management-ui";

function pageFrom(value: string | null): number {
  if (!value || !/^[1-9][0-9]*$/u.test(value)) {
    return defaultGatewayListRequest.page;
  }
  const page = Number(value);
  return Number.isSafeInteger(page) ? page : defaultGatewayListRequest.page;
}

function sortDirectionFrom(value: string | null) {
  return value === "asc" || value === "desc"
    ? value
    : defaultGatewayListRequest.sortDirection;
}

function sizeFrom(value: string | null): number {
  if (!value || !/^[1-9][0-9]*$/u.test(value)) {
    return defaultGatewayListRequest.size;
  }
  const size = Number(value);
  return gatewayListPageSizes.some((candidate) => candidate === size)
    ? size
    : defaultGatewayListRequest.size;
}

function sortFieldFrom(value: string | null) {
  switch (value) {
    case "cluster":
    case "created":
    case "endpoint":
    case "name":
    case "owner":
    case "status":
      return value;
    default:
      return defaultGatewayListRequest.sortField;
  }
}

export function parseGatewayListState(
  parameters: URLSearchParams,
): GatewayListRequest {
  const sort = parameters.get("sort");
  const direction = parameters.get("direction");
  return {
    page: pageFrom(parameters.get("page")),
    search: parameters.get("q") ?? defaultGatewayListRequest.search,
    size: sizeFrom(parameters.get("size")),
    sortDirection: sortDirectionFrom(direction),
    sortField: sortFieldFrom(sort),
  };
}

export function serializeGatewayListState(
  state: GatewayListRequest,
): URLSearchParams {
  const parameters = new URLSearchParams();
  if (state.search) {
    parameters.set("q", state.search);
  }
  if (state.page !== defaultGatewayListRequest.page) {
    parameters.set("page", String(state.page));
  }
  if (state.size !== defaultGatewayListRequest.size) {
    parameters.set("size", String(state.size));
  }
  if (state.sortField !== defaultGatewayListRequest.sortField) {
    parameters.set("sort", state.sortField);
  }
  if (state.sortDirection !== defaultGatewayListRequest.sortDirection) {
    parameters.set("direction", state.sortDirection);
  }
  return parameters;
}
