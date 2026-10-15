import type { FastifyInstance } from "fastify";

// Browsers drop any cookie whose name plus value exceeds 4096 bytes, without
// an error. The encrypted session cookies carry the IdP access token, whose
// size grows with the user's roles (for example one client role per gateway),
// so a single cookie can cross the limit. When that happens the browser keeps
// the stale pre-login cookie and every callback loops back to /auth/login.
//
// Chunking keeps each cookie small: a value longer than the chunk budget is
// written as `<name>`, `<name>.1`, `<name>.2`, and so on, and the chunks are
// joined again before the session plugin decodes the cookie.
const DEFAULT_MAX_CHUNK_BYTES = 3800;

/**
 * Splits a cookie value so each chunk stays within `maxBytes` once
 * URL-encoded, which is how @fastify/cookie serializes values.
 */
export function splitCookieValue(value: string, maxBytes: number): string[] {
  const chunks: string[] = [];
  let current = "";
  let currentBytes = 0;
  for (const char of value) {
    const charBytes = encodeURIComponent(char).length;
    if (currentBytes + charBytes > maxBytes && current !== "") {
      chunks.push(current);
      current = "";
      currentBytes = 0;
    }
    current += char;
    currentBytes += charBytes;
  }
  chunks.push(current);
  return chunks;
}

function continuationName(name: string, index: number): string {
  return `${name}.${String(index)}`;
}

interface JoinedCookieHeader {
  /** The Cookie header with each chunked cookie joined into one value. */
  readonly header: string;
  /** How many chunks the client sent for each chunked cookie name. */
  readonly counts: ReadonlyMap<string, number>;
}

/**
 * Joins `<name>.N` continuation cookies into their primary cookie. Values
 * stay URL-encoded: chunks split on character boundaries, so joining the
 * encoded chunks yields the encoded original value.
 */
export function joinCookieChunks(
  header: string,
  cookieNames: ReadonlySet<string>,
): JoinedCookieHeader {
  const pairs = header
    .split(";")
    .map((pair) => pair.trim())
    .filter((pair) => pair !== "")
    .map((pair) => {
      const separator = pair.indexOf("=");
      return separator === -1
        ? { name: pair, value: "" }
        : { name: pair.slice(0, separator), value: pair.slice(separator + 1) };
    });
  const values = new Map(pairs.map(({ name, value }) => [name, value]));

  const counts = new Map<string, number>();
  const continuations = new Set<string>();
  const joined = new Map<string, string>();
  for (const name of cookieNames) {
    let count = 1;
    let value = values.get(name) ?? "";
    for (
      let part = values.get(continuationName(name, count));
      part !== undefined;
      part = values.get(continuationName(name, count))
    ) {
      continuations.add(continuationName(name, count));
      value += part;
      count += 1;
    }
    counts.set(name, count);
    if (values.has(name) && count > 1) {
      joined.set(name, value);
    }
  }

  if (continuations.size === 0) {
    return { counts, header };
  }
  return {
    counts,
    header: pairs
      .filter(({ name }) => !continuations.has(name))
      .map(({ name, value }) => `${name}=${joined.get(name) ?? value}`)
      .join("; "),
  };
}

/**
 * Transparently chunks the named cookies across `<name>.N` continuation
 * cookies.
 *
 * Call it before registering the plugin that parses and reads the cookies
 * (@fastify/secure-session registers @fastify/cookie itself), so this request
 * hook rewrites the Cookie header before it is parsed.
 */
export function registerCookieChunking(
  app: FastifyInstance,
  cookieNames: readonly string[],
  maxChunkBytes: number = DEFAULT_MAX_CHUNK_BYTES,
): void {
  const chunked: ReadonlySet<string> = new Set(cookieNames);

  app.addHook("onRequest", async (request, reply) => {
    const raw = request.raw.headers.cookie;
    let counts: ReadonlyMap<string, number> = new Map();
    if (raw !== undefined) {
      const result = joinCookieChunks(raw, chunked);
      request.raw.headers.cookie = result.header;
      counts = result.counts;
    }

    // Split oversized values on the way out, and expire continuation chunks
    // the browser still holds from a longer previous value.
    const setCookie = reply.setCookie.bind(reply);
    reply.setCookie = (name, value, options) => {
      if (!chunked.has(name)) {
        return setCookie(name, value, options);
      }
      const chunks = splitCookieValue(value, maxChunkBytes);
      chunks.forEach((chunk, index) => {
        setCookie(
          index === 0 ? name : continuationName(name, index),
          chunk,
          options,
        );
      });
      const previousCount = counts.get(name) ?? 1;
      for (let index = chunks.length; index < previousCount; index += 1) {
        setCookie(continuationName(name, index), "", {
          ...options,
          expires: new Date(0),
          maxAge: 0,
        });
      }
      return reply;
    };
  });
}
