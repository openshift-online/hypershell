import {
  index,
  layout,
  route,
  type RouteConfig,
} from "@react-router/dev/routes";

export default [
  route("login", "./routes/login.tsx"),
  layout("./routes/application.tsx", [
    index("./routes/home.tsx"),
    route("gateways/new", "./routes/gateway-new.tsx"),
    route("gateways/:gatewayId", "./routes/gateway.tsx"),
  ]),
  route("*", "./routes/not-found.tsx"),
] satisfies RouteConfig;
