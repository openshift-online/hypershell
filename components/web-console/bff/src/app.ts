import { createHash } from "node:crypto";
import { readFile } from "node:fs/promises";
import path from "node:path";

import compress from "@fastify/compress";
import helmet from "@fastify/helmet";
import fastifyStatic from "@fastify/static";
import Fastify, { type FastifyInstance, LogController } from "fastify";

import type { ServerConfig } from "./config.js";

const applicationRoutes = [
  "/",
  "/login",
  "/fleets",
  "/fleets/:fleetId",
  "/fleets/:fleetId/gateways",
  "/fleets/:fleetId/gateways/:gatewayId",
  "/fleets/:fleetId/clients",
  "/fleets/:fleetId/keys",
  "/fleets/:fleetId/settings",
] as const;

function inlineScriptHashes(document: string): string[] {
  const hashes = new Set<string>();
  const scriptPattern = /<script\b(?![^>]*\bsrc=)[^>]*>([\s\S]*?)<\/script>/giu;

  for (const match of document.matchAll(scriptPattern)) {
    const body = match[1];
    if (body) {
      hashes.add(
        `'sha256-${createHash("sha256").update(body).digest("base64")}'`,
      );
    }
  }

  return [...hashes];
}

export async function buildApp(config: ServerConfig): Promise<FastifyInstance> {
  const indexPath = path.join(config.staticRoot, "index.html");
  const indexDocument = await readFile(indexPath, "utf8");
  const scriptHashes = inlineScriptHashes(indexDocument);

  const app = Fastify({
    bodyLimit: 1_048_576,
    connectionTimeout: 10_000,
    keepAliveTimeout: 72_000,
    logController: new LogController({ disableRequestLogging: true }),
    logger: {
      level: config.logLevel,
      redact: {
        paths: [
          "req.headers.authorization",
          "req.headers.cookie",
          "res.headers.set-cookie",
        ],
        censor: "[REDACTED]",
      },
    },
    requestTimeout: 30_000,
    trustProxy: false,
  });

  await app.register(helmet, {
    contentSecurityPolicy: {
      directives: {
        defaultSrc: ["'none'"],
        baseUri: ["'none'"],
        connectSrc: ["'self'"],
        fontSrc: ["'self'"],
        formAction: ["'self'"],
        frameAncestors: ["'none'"],
        imgSrc: ["'self'", "data:"],
        objectSrc: ["'none'"],
        scriptSrc: ["'self'", ...scriptHashes],
        styleSrc: ["'self'"],
        styleSrcAttr: ["'none'"],
        // The BFF serves plain HTTP behind the deployment TLS terminator.
        // HTTPS already blocks mixed active content, while this directive
        // breaks direct local HTTP use in WebKit by upgrading relative assets.
        upgradeInsecureRequests: null,
      },
    },
    crossOriginEmbedderPolicy: false,
    referrerPolicy: { policy: "no-referrer" },
  });

  app.addHook("onSend", async (_request, reply) => {
    reply.header(
      "Permissions-Policy",
      "camera=(), geolocation=(), microphone=(), payment=(), usb=()",
    );
  });

  app.addHook("onResponse", async (request, reply) => {
    request.log.info(
      {
        method: request.method,
        requestId: request.id,
        route: request.routeOptions.url,
        statusCode: reply.statusCode,
      },
      "request complete",
    );
  });

  await app.register(compress, {
    global: true,
    threshold: 1024,
  });

  await app.register(fastifyStatic, {
    root: path.join(config.staticRoot, "assets"),
    prefix: "/assets/",
    decorateReply: false,
    immutable: true,
    maxAge: "1y",
    index: false,
    redirect: false,
  });

  app.get("/health/live", async (_request, reply) => {
    reply.header("Cache-Control", "no-store");
    return { status: "ok" };
  });

  app.get("/health/ready", async (_request, reply) => {
    reply.header("Cache-Control", "no-store");
    return { status: "ready" };
  });

  const sendApplication = (
    _request: unknown,
    reply: {
      header(name: string, value: string): unknown;
      type(value: string): { send(payload: string): unknown };
    },
  ) => {
    reply.header("Cache-Control", "no-store");
    return reply.type("text/html; charset=utf-8").send(indexDocument);
  };

  for (const applicationRoute of applicationRoutes) {
    app.get(applicationRoute, sendApplication);
  }

  app.all("/api/*", async (_request, reply) => {
    reply.code(404);
    return { error: "Not Found", statusCode: 404 };
  });

  app.setNotFoundHandler(async (_request, reply) => {
    reply.code(404);
    return { error: "Not Found", statusCode: 404 };
  });

  app.setErrorHandler(async (error, request, reply) => {
    request.log.error({ err: error }, "request failed");
    const errorStatus =
      typeof error === "object" &&
      error !== null &&
      "statusCode" in error &&
      typeof error.statusCode === "number"
        ? error.statusCode
        : 500;
    reply.code(errorStatus >= 400 && errorStatus < 500 ? errorStatus : 500);
    return {
      error:
        reply.statusCode < 500 ? "Request failed" : "Internal Server Error",
      statusCode: reply.statusCode,
    };
  });

  return app;
}
