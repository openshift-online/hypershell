import { index, route, type RouteConfig } from "@react-router/dev/routes";

export default [
  index("./routes/home.tsx"),
  route("login", "./routes/login.tsx"),
  route("fleets", "./routes/fleets.tsx"),
  route("fleets/:fleetId", "./routes/fleet.tsx"),
  route("fleets/:fleetId/gateways", "./routes/fleet-gateways.tsx"),
  route("fleets/:fleetId/gateways/:gatewayId", "./routes/gateway.tsx"),
  route("fleets/:fleetId/clients", "./routes/fleet-clients.tsx"),
  route("fleets/:fleetId/keys", "./routes/fleet-keys.tsx"),
  route("fleets/:fleetId/settings", "./routes/fleet-settings.tsx"),
  route("*", "./routes/not-found.tsx"),
] satisfies RouteConfig;
