import {
  index,
  layout,
  route,
  type RouteConfig,
} from "@react-router/dev/routes";

import { routes } from "../shared/routes";

export default [
  route(routes.login, "./routes/login.tsx"),
  layout("./routes/application.tsx", [
    index("./routes/home.tsx"),
    route(routes.dashboard, "./routes/dashboard.tsx"),
    route(routes.dashboardProvisioning, "./routes/dashboard-provisioning.tsx"),
    route(routes.dashboardInventory, "./routes/dashboard-inventory.tsx"),
    route(routes.dashboardReliability, "./routes/dashboard-reliability.tsx"),
    route(routes.dashboardUsage, "./routes/dashboard-usage.tsx"),
    route(routes.dashboardAdoption, "./routes/dashboard-adoption.tsx"),
    route(routes.gatewayList, "./routes/gateways.tsx"),
    route(routes.gatewayNew, "./routes/gateway-new.tsx"),
    route(routes.gatewayDetail, "./routes/gateway.tsx"),
    route(routes.metrics, "./routes/metrics.tsx"),
  ]),
  route("*", "./routes/not-found.tsx"),
] satisfies RouteConfig;
