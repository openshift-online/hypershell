import {
  defaultGatewayProfileListRequest,
  gatewayProfileListPageSizes,
  type GatewayProfileListRequest,
} from "@openshift-online/hypershell-gateway-management-ui";

function pageFrom(value: string | null): number {
  if (!value || !/^[1-9][0-9]*$/u.test(value)) {
    return defaultGatewayProfileListRequest.page;
  }
  const page = Number(value);
  return Number.isSafeInteger(page)
    ? page
    : defaultGatewayProfileListRequest.page;
}

function sortDirectionFrom(value: string | null) {
  return value === "asc" || value === "desc"
    ? value
    : defaultGatewayProfileListRequest.sortDirection;
}

function sizeFrom(value: string | null): number {
  if (!value || !/^[1-9][0-9]*$/u.test(value)) {
    return defaultGatewayProfileListRequest.size;
  }
  const size = Number(value);
  return gatewayProfileListPageSizes.some((candidate) => candidate === size)
    ? size
    : defaultGatewayProfileListRequest.size;
}

function sortFieldFrom(value: string | null) {
  switch (value) {
    case "created":
    case "name":
      return value;
    default:
      return defaultGatewayProfileListRequest.sortField;
  }
}

export function parseGatewayProfileListState(
  parameters: URLSearchParams,
): GatewayProfileListRequest {
  const sort = parameters.get("sort");
  const direction = parameters.get("direction");
  return {
    page: pageFrom(parameters.get("page")),
    search: parameters.get("q") ?? defaultGatewayProfileListRequest.search,
    size: sizeFrom(parameters.get("size")),
    sortDirection: sortDirectionFrom(direction),
    sortField: sortFieldFrom(sort),
  };
}

export function serializeGatewayProfileListState(
  state: GatewayProfileListRequest,
): URLSearchParams {
  const parameters = new URLSearchParams();
  if (state.search) {
    parameters.set("q", state.search);
  }
  if (state.page !== defaultGatewayProfileListRequest.page) {
    parameters.set("page", String(state.page));
  }
  if (state.size !== defaultGatewayProfileListRequest.size) {
    parameters.set("size", String(state.size));
  }
  if (state.sortField !== defaultGatewayProfileListRequest.sortField) {
    parameters.set("sort", state.sortField);
  }
  if (state.sortDirection !== defaultGatewayProfileListRequest.sortDirection) {
    parameters.set("direction", state.sortDirection);
  }
  return parameters;
}
