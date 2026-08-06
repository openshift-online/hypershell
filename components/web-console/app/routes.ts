import {
  index,
  layout,
  route,
  type RouteConfig,
} from "@react-router/dev/routes";

export default [
  route("login", "./routes/login.tsx"),
  layout("./routes/user-application.tsx", [
    index("./routes/home.tsx"),
    route("gateways/:gatewayId", "./routes/gateway.tsx"),
  ]),
  layout("./routes/admin-application.tsx", [
    route("admin", "./routes/admin-home.tsx"),
    route("admin/gateways/new", "./routes/admin-gateway-new.tsx"),
    route("admin/gateways/:gatewayId", "./routes/admin-gateway.tsx"),
  ]),
  route("*", "./routes/not-found.tsx"),
] satisfies RouteConfig;
