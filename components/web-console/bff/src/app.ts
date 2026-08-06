import { createHash, randomUUID } from "node:crypto";
import { readFile } from "node:fs/promises";
import path from "node:path";

import compress from "@fastify/compress";
import helmet from "@fastify/helmet";
import fastifyStatic from "@fastify/static";
import Fastify, { type FastifyInstance, LogController } from "fastify";

import type { ServerConfig } from "./config.js";

const correlationHeader = "x-hypershell-correlation-id";
const validCorrelationId =
  /^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/iu;
const forwardedRequestHeaders = [
  "accept",
  "content-type",
  "if-match",
  "if-none-match",
] as const;
const forwardedResponseHeaders = [
  "content-type",
  "etag",
  "location",
  "retry-after",
] as const;

declare module "fastify" {
  interface FastifyRequest {
    correlationId: string;
  }
}

function isApplicationRoute(pathname: string): boolean {
  return (
    pathname === "/" ||
    pathname === "/login" ||
    pathname === "/gateways/new" ||
    /^\/gateways\/[^/]+\/?$/u.test(pathname)
  );
}

function proxyBody(
  method: string,
  body: unknown,
): string | Uint8Array | undefined {
  if (["GET", "HEAD"].includes(method) || body === undefined) {
    return undefined;
  }
  if (typeof body === "string" || body instanceof Uint8Array) {
    return body;
  }
  return JSON.stringify(body);
}

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

  app.decorateRequest("correlationId", "");

  app.addHook("onRequest", async (request, reply) => {
    const supplied = request.headers[correlationHeader];
    request.correlationId =
      typeof supplied === "string" && validCorrelationId.test(supplied)
        ? supplied
        : randomUUID();
    reply.header(correlationHeader, request.correlationId);
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
        correlationId: request.correlationId,
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

  app.all("/api/*", async (request, reply) => {
    const incoming = new URL(request.url, "http://bff.invalid");
    const target = new URL(
      `${incoming.pathname}${incoming.search}`,
      config.apiOrigin,
    );
    const headers = new Headers();
    for (const header of forwardedRequestHeaders) {
      const value = request.headers[header];
      if (typeof value === "string") {
        headers.set(header, value);
      }
    }
    headers.set(correlationHeader, request.correlationId);

    const controller = new AbortController();
    const timeoutReason = new Error("Upstream API request timed out");
    const timeout = setTimeout(() => {
      controller.abort(timeoutReason);
    }, config.apiTimeoutMs);
    const abortDownstream = () => {
      controller.abort();
    };
    request.raw.once("aborted", abortDownstream);

    try {
      const upstream = await fetch(target, {
        body: proxyBody(request.method, request.body),
        headers,
        method: request.method,
        redirect: "manual",
        signal: controller.signal,
      });

      for (const header of forwardedResponseHeaders) {
        const value = upstream.headers.get(header);
        if (value !== null) {
          reply.header(header, value);
        }
      }
      reply.code(upstream.status);
      if (request.method === "HEAD" || upstream.status === 204) {
        return await reply.send();
      }
      return await reply.send(Buffer.from(await upstream.arrayBuffer()));
    } catch (error) {
      if (error === timeoutReason) {
        reply.code(504);
        return {
          error: "Gateway Timeout",
          statusCode: 504,
        };
      }
      throw error;
    } finally {
      clearTimeout(timeout);
      request.raw.off("aborted", abortDownstream);
    }
  });

  app.setNotFoundHandler(async (request, reply) => {
    if (
      request.method === "GET" &&
      isApplicationRoute(new URL(request.url, "http://localhost").pathname)
    ) {
      return sendApplication(request, reply);
    }
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
