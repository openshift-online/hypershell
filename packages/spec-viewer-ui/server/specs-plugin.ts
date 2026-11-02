import type { IncomingMessage, ServerResponse } from "node:http";
import path from "node:path";
import type { Plugin } from "vite";

import { isSpecFile, readSpecFiles, specFilesFromTree } from "./read-specs";
import {
  createRepository,
  findMarkers,
  findReferences,
  ID_PATTERN,
  readText,
  repositoryRoot,
  UnknownRefError,
  WORKING_TREE,
  type Repository,
} from "./repository";

const CHANGED_EVENT = "spec-viewer:specs-changed";

function sendJson(response: ServerResponse, body: unknown): void {
  response.setHeader("Content-Type", "application/json");
  response.setHeader("Cache-Control", "no-store");
  response.end(JSON.stringify(body));
}

function sendError(
  response: ServerResponse,
  status: number,
  message: string,
): void {
  response.statusCode = status;
  response.setHeader("Content-Type", "text/plain");
  response.end(message);
}

const errorText = (error: unknown): string =>
  error instanceof Error ? error.message : String(error);

const params = (request: IncomingMessage): URLSearchParams =>
  new URL(request.url ?? "", "http://local").searchParams;

/**
 * Serves the viewer's data API from this checkout (dev server only). A build
 * never contains spec data: the specs mention fleet names and the fleet
 * dashboard image that embeds the build is public.
 *
 * Every endpoint except `refs` takes `ref`: "@" (the default) is the working
 * tree, with uncommitted edits and a reload event when a spec changes; any
 * other value is a local branch, read from git.
 *
 * - `/api/refs` lists "@" and the local branches.
 * - `/api/specs.json` returns every markdown file under the specs folder.
 * - `/api/references?id=ABC-DEF-01` lists repository lines that mention the ID.
 * - `/api/file?path=...` returns one file. Only files git lists can be read.
 * - `/api/markers` returns every `Verifies:` test marker line; the viewer
 *   applies the marker grammar.
 *
 * The fleet dashboard serves the same endpoints from Go
 * (components/fleet-dashboard/pkg/specs); keep the two in step.
 */
export function specsPlugin(specsRoot: string): Plugin {
  return {
    name: "hypershell-spec-viewer-specs",
    configureServer(server) {
      let root: Promise<string> | undefined;
      let repository: Promise<Repository> | undefined;
      const getRoot = () => (root ??= repositoryRoot(specsRoot));
      const getRepository = () =>
        (repository ??= getRoot().then(createRepository));

      server.watcher.add(specsRoot);
      const notify = (file: string) => {
        if (isSpecFile(specsRoot, file)) {
          server.ws.send({ type: "custom", event: CHANGED_EVENT });
        }
      };
      server.watcher.on("add", notify);
      server.watcher.on("change", notify);
      server.watcher.on("unlink", notify);

      /** Runs `work` on the tree of the request's ref and sends the answer. */
      const serve = (
        path_: string,
        what: string,
        work: (
          request: IncomingMessage,
          ref: string,
          repository: Repository,
        ) => Promise<unknown>,
      ) => {
        server.middlewares.use(path_, (request, response) => {
          const requested = params(request).get("ref");
          // An empty ref is the default, as in the fleet dashboard.
          const ref =
            requested === null || requested === "" ? WORKING_TREE : requested;
          getRepository()
            .then((repo) => work(request, ref, repo))
            .then(
              (body) => {
                if (body === null) {
                  sendError(response, 404, `${what}: not found`);
                } else {
                  sendJson(response, body);
                }
              },
              (error: unknown) => {
                sendError(
                  response,
                  error instanceof UnknownRefError ? 404 : 500,
                  `${what} failed: ${errorText(error)}`,
                );
              },
            );
        });
      };

      server.middlewares.use("/api/refs", (_request, response) => {
        getRepository()
          .then((repo) => repo.listRefs())
          .then(
            (refs) => {
              sendJson(response, refs);
            },
            (error: unknown) => {
              sendError(
                response,
                500,
                `Listing branches failed: ${errorText(error)}`,
              );
            },
          );
      });

      server.middlewares.use("/api/references", (request, response, next) => {
        const id = params(request).get("id");
        if (!id || !ID_PATTERN.test(id)) {
          sendError(
            response,
            400,
            `Not a requirement or scenario ID: ${id ?? ""}`,
          );
          return;
        }
        next();
      });
      serve("/api/references", "Searching", async (request, ref, repo) =>
        findReferences(await repo.tree(ref), params(request).get("id") ?? ""),
      );

      serve(
        "/api/markers",
        "Scanning for test markers",
        async (_r, ref, repo) => findMarkers(await repo.tree(ref)),
      );

      server.middlewares.use("/api/file", (request, response, next) => {
        if (!params(request).get("path")) {
          sendError(response, 400, "Missing path");
          return;
        }
        next();
      });
      serve("/api/file", "Reading the file", async (request, ref, repo) => {
        const file = params(request).get("path") ?? "";
        const content = await readText(await repo.tree(ref), file);
        return content === null ? null : { path: file, content };
      });

      serve("/api/specs.json", "Reading specs", async (_r, ref, repo) => {
        if (ref === WORKING_TREE) return readSpecFiles(specsRoot);
        const prefix = path.relative(await getRoot(), specsRoot);
        if (prefix.startsWith("..") || path.isAbsolute(prefix)) {
          throw new Error(
            `${specsRoot} is outside the repository; only "@" can be read`,
          );
        }
        return specFilesFromTree(
          await repo.tree(ref),
          prefix === "" ? "" : `${prefix.split(path.sep).join("/")}/`,
        );
      });
    },
  };
}
