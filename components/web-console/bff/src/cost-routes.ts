import type {
  FastifyInstance,
  FastifyReply,
  FastifyRequest,
  preHandlerAsyncHookHandler,
} from "fastify";

import {
  listCostGateways,
  listMemberWorkspaces,
  resolveGatewayAccess,
  type CostGateway,
} from "./cost-attribution.js";
import type { RateCardSource } from "./cost-rate-card.js";
import type { ServerConfig } from "./config.js";
import {
  buildBreakdownResponse,
  buildGatewaysResponse,
  buildMyCostGateways,
  buildMyCostSummary,
  buildSummaryResponse,
  CostSeriesMissingError,
  queryActiveGatewayNamespaces,
  queryActiveUsers,
  queryMeanActiveSandboxes,
  queryPlatformDaily,
  queryPlatformSnapshot,
  queryScopedDaily,
  queryScopedGatewayCost,
  type CostContext,
  type CostRange,
  type MyCostGatewayInput,
} from "./metrics-cost.js";

export interface CostRouteDependencies {
  config: ServerConfig;
  freshAccessToken: (request: FastifyRequest) => Promise<string | undefined>;
  rateCards: RateCardSource;
  requireAdmin: preHandlerAsyncHookHandler;
  requireSignedIn: preHandlerAsyncHookHandler;
  respondReauth: (reply: FastifyReply) => unknown;
}

function costRange(request: FastifyRequest): CostRange {
  const query = request.query as { range?: unknown };
  return query.range === "30" ? 30 : 7;
}

/**
 * Cost dashboard routes (WEB-COST-02, WEB-COST-03, WEB-COST-09). Platform
 * routes are admin-only; `me/*` routes serve every signed-in user and are
 * scoped to the caller's own gateway bindings.
 */
export function registerCostRoutes(
  app: FastifyInstance,
  deps: CostRouteDependencies,
): void {
  const { config, rateCards } = deps;

  const costContext = (): CostContext | undefined => {
    const state = rateCards.current();
    if (state.status !== "ready") return undefined;
    return {
      costSource: {
        url: config.costPrometheusUrl ?? config.prometheusUrl,
        tokenFile: config.costPrometheusTokenFile,
        caFile: config.costPrometheusCaFile,
      },
      options: {
        clusters: config.costClusters,
        usageNamespace: config.prometheusNamespace,
        usageSource: config.prometheusUrl,
      },
      rateCard: state.rateCard,
      timeoutMs: config.prometheusQueryTimeoutMs,
    };
  };

  const notConfigured = (reply: FastifyReply) => {
    const state = rateCards.current();
    if (state.status === "invalid") {
      reply.code(503);
      return { error: "Cost model unavailable", statusCode: 503 };
    }
    // WEB-COST-03.01: a missing rate card is a state, not a $0 estimate.
    return { status: "not-configured", estimate: true };
  };

  const unavailable = (
    request: FastifyRequest,
    reply: FastifyReply,
    error: unknown,
    message: string,
  ) => {
    request.log.warn(
      { err: error, missingSeries: error instanceof CostSeriesMissingError },
      message,
    );
    reply.code(502);
    return { error: "Metrics unavailable", statusCode: 502 };
  };

  const adminGateways = async (request: FastifyRequest) => {
    const accessToken = await deps.freshAccessToken(request);
    const gateways = await listCostGateways(
      config.apiOrigin,
      accessToken,
      config.apiTimeoutMs,
    );
    return { accessToken, gateways };
  };

  const billingOwners = async (
    accessToken: string | undefined,
    gateways: readonly CostGateway[],
  ) => {
    const access = await resolveGatewayAccess(
      config.apiOrigin,
      accessToken,
      config.apiTimeoutMs,
      gateways,
    );
    return new Map(
      [...access].map(([id, value]) => [id, value.billingOwner] as const),
    );
  };

  app.get(
    "/api/metrics/cost/summary",
    { preHandler: deps.requireAdmin },
    async (request, reply) => {
      const context = costContext();
      if (!context) return notConfigured(reply);
      try {
        const range = costRange(request);
        const { gateways } = await adminGateways(request);
        const [snapshot, daily, activeNamespaces, activeUsers, sandboxes] =
          await Promise.all([
            queryPlatformSnapshot(context, range, gateways),
            queryPlatformDaily(context, range).catch(() => undefined),
            queryActiveGatewayNamespaces(context, range),
            queryActiveUsers(context, range),
            queryMeanActiveSandboxes(context, range),
          ]);
        return buildSummaryResponse(context.rateCard, range, snapshot, {
          activeNamespaces,
          activeUsers,
          currentGatewayCount: gateways.length,
          daily: daily?.idle,
          meanActiveSandboxes: sandboxes,
        });
      } catch (error) {
        return unavailable(request, reply, error, "cost summary query failed");
      }
    },
  );

  app.get(
    "/api/metrics/cost/breakdown",
    { preHandler: deps.requireAdmin },
    async (request, reply) => {
      const context = costContext();
      if (!context) return notConfigured(reply);
      try {
        const range = costRange(request);
        const { accessToken, gateways } = await adminGateways(request);
        const [snapshot, daily, owners] = await Promise.all([
          queryPlatformSnapshot(context, range, gateways),
          queryPlatformDaily(context, range).catch(() => undefined),
          billingOwners(accessToken, gateways).catch(() => undefined),
        ]);
        return buildBreakdownResponse(context.rateCard, range, snapshot, {
          billingOwners: owners,
          daily,
        });
      } catch (error) {
        return unavailable(
          request,
          reply,
          error,
          "cost breakdown query failed",
        );
      }
    },
  );

  app.get(
    "/api/metrics/cost/gateways",
    { preHandler: deps.requireAdmin },
    async (request, reply) => {
      const context = costContext();
      if (!context) return notConfigured(reply);
      try {
        const range = costRange(request);
        const { accessToken, gateways } = await adminGateways(request);
        const [snapshot, activeNamespaces] = await Promise.all([
          queryPlatformSnapshot(context, range, gateways),
          queryActiveGatewayNamespaces(context, range),
        ]);
        const top = [...snapshot.gateways.values()]
          .sort((left, right) => right.cost - left.cost)
          .slice(0, 10)
          .map((row) => row.gateway);
        const owners = await billingOwners(accessToken, top).catch(
          () => undefined,
        );
        return buildGatewaysResponse(context.rateCard, range, snapshot, {
          activeNamespaces,
          billingOwners: owners,
        });
      } catch (error) {
        return unavailable(request, reply, error, "cost gateways query failed");
      }
    },
  );

  /**
   * The caller's own gateway bindings (WEB-COST-09 step 1). A platform admin
   * gets only gateways where they hold a binding, not the whole fleet.
   */
  const myGateways = async (
    request: FastifyRequest,
    accessToken: string | undefined,
  ): Promise<MyCostGatewayInput[]> => {
    const gateways = await listCostGateways(
      config.apiOrigin,
      accessToken,
      config.apiTimeoutMs,
    );
    const username = config.oidcIssuer
      ? request.session.get("preferredUsername")
      : undefined;
    if (config.oidcIssuer && username === undefined) {
      throw new Error("Signed-in session has no username");
    }
    const access = await resolveGatewayAccess(
      config.apiOrigin,
      accessToken,
      config.apiTimeoutMs,
      gateways,
      { username },
    );
    const inputs: MyCostGatewayInput[] = [];
    for (const gateway of gateways) {
      const entry = access.get(gateway.id);
      if (entry?.callerRole === undefined) continue;
      inputs.push({
        gateway,
        callerRole: entry.callerRole,
        billingOwner: entry.billingOwner,
        isBillingOwner:
          entry.billingOwner !== undefined &&
          (username === undefined ||
            entry.billingOwner.username.toLowerCase() ===
              username.toLowerCase()),
      });
    }
    return inputs;
  };

  const myCostPrelude = async (
    request: FastifyRequest,
    reply: FastifyReply,
  ) => {
    const accessToken = await deps.freshAccessToken(request);
    if (config.oidcIssuer && accessToken === undefined) {
      return { reauth: deps.respondReauth(reply) } as const;
    }
    const inputs = await myGateways(request, accessToken);
    return { accessToken, inputs } as const;
  };

  app.get(
    "/api/metrics/cost/me/summary",
    { preHandler: deps.requireSignedIn },
    async (request, reply) => {
      const context = costContext();
      if (!context) return notConfigured(reply);
      try {
        const range = costRange(request);
        const prelude = await myCostPrelude(request, reply);
        if ("reauth" in prelude) return prelude.reauth;
        const { inputs } = prelude;
        const gateways = inputs.map((input) => input.gateway);
        const charged = inputs
          .filter((input) => input.isBillingOwner)
          .map((input) => input.gateway);
        const [scoped, trend] = await Promise.all([
          gateways.length > 0
            ? queryScopedGatewayCost(context, range, gateways)
            : Promise.resolve({
                costs: new Map(),
                coveredHours: 1,
                unpricedClusters: [],
              }),
          queryScopedDaily(context, range, charged),
        ]);
        return buildMyCostSummary(
          context.rateCard,
          range,
          inputs,
          scoped,
          trend,
        );
      } catch (error) {
        return unavailable(
          request,
          reply,
          error,
          "my cost summary query failed",
        );
      }
    },
  );

  app.get(
    "/api/metrics/cost/me/gateways",
    { preHandler: deps.requireSignedIn },
    async (request, reply) => {
      const context = costContext();
      if (!context) return notConfigured(reply);
      try {
        const range = costRange(request);
        const prelude = await myCostPrelude(request, reply);
        if ("reauth" in prelude) return prelude.reauth;
        const { accessToken, inputs } = prelude;
        const gateways = inputs.map((input) => input.gateway);
        const namespaces = gateways.map((gateway) => gateway.namespace);
        const [scoped, activeNamespaces, memberWorkspaces] = await Promise.all([
          gateways.length > 0
            ? queryScopedGatewayCost(context, range, gateways)
            : Promise.resolve({
                costs: new Map(),
                coveredHours: 1,
                unpricedClusters: [],
              }),
          gateways.length > 0
            ? queryActiveGatewayNamespaces(context, range, namespaces)
            : Promise.resolve(new Set<string>()),
          listMemberWorkspaces(
            config.apiOrigin,
            accessToken,
            config.apiTimeoutMs,
            config.oidcIssuer ? request.session.get("sub") : undefined,
          ),
        ]);
        // WEB-COST-09 step 4: workspace rows only on gateways the caller owns,
        // or for workspaces where the caller is a member.
        const visible = (input: MyCostGatewayInput) => {
          if (input.callerRole === "owner") return () => true;
          const names = memberWorkspaces.get(input.gateway.id);
          return (workspace: string) => names?.has(workspace) === true;
        };
        return buildMyCostGateways(
          context.rateCard,
          range,
          inputs,
          scoped,
          visible,
          activeNamespaces,
        );
      } catch (error) {
        return unavailable(
          request,
          reply,
          error,
          "my cost gateways query failed",
        );
      }
    },
  );
}
