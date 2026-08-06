import {
  index,
  layout,
  route,
  type RouteConfig,
} from "@react-router/dev/routes";

import routeContract from "../route-contract.json";

export default [
  route(routeContract.login, "./routes/login.tsx"),
  layout("./routes/application.tsx", [
    index("./routes/home.tsx"),
    route(routeContract.gatewayNew, "./routes/gateway-new.tsx"),
    route(routeContract.gatewayDetail, "./routes/gateway.tsx"),
  ]),
  route("*", "./routes/not-found.tsx"),
] satisfies RouteConfig;
