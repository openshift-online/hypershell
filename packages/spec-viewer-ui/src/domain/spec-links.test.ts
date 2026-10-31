import { markedMarkdown } from "../adapters/marked-markdown";
import { buildCatalog, parseSpecFile } from "./spec-document";
import {
  githubSlug,
  resolvePath,
  resolveSpecLink,
  resolveSpecReference,
} from "./spec-links";

const catalog = buildCatalog(
  [
    [
      "platform/e2e.spec.md",
      "# E2E\n\n## Environment Variables\n\nx\n\n### Requirement: Opt-in Structured Run Reporting\n\ny\n\n## Notes\n\na\n\n## Notes\n\nb\n",
    ],
    ["platform/data-model.spec.md", "# Data model\n"],
    ["web-console/data-model.spec.md", "# Console data model\n"],
    ["standards/ui/patternfly.spec.md", "# PatternFly\n"],
  ].map(([path = "", content = ""]) =>
    parseSpecFile(path, markedMarkdown.lex(content)),
  ),
);

describe("githubSlug", () => {
  it("matches GitHub heading anchors", () => {
    expect(githubSlug("Requirement: QRX-01 -- Provision `Time`")).toBe(
      "requirement-qrx-01----provision-time",
    );
  });
});

describe("resolvePath", () => {
  it("joins relative paths and refuses to leave the root", () => {
    expect(resolvePath("platform/e2e.spec.md", "../standards/ui/x.md")).toBe(
      "standards/ui/x.md",
    );
    expect(resolvePath("platform/e2e.spec.md", "./a.md")).toBe("platform/a.md");
    expect(resolvePath("platform/e2e.spec.md", "/web-console/a.md")).toBe(
      "web-console/a.md",
    );
    expect(resolvePath("platform/e2e.spec.md", "../../scripts/x")).toBeNull();
  });
});

describe("resolveSpecLink", () => {
  const from = "platform/e2e.spec.md";

  it("opens linked spec files at their first heading", () => {
    expect(
      resolveSpecLink(catalog, from, "../standards/ui/patternfly.spec.md"),
    ).toEqual({
      kind: "spec",
      nodeId: "standards/ui/patternfly.spec.md#patternfly",
    });
  });

  it("resolves heading anchors, including duplicates and same-file links", () => {
    expect(
      resolveSpecLink(
        catalog,
        "platform/data-model.spec.md",
        "e2e.spec.md#requirement-opt-in-structured-run-reporting",
      ),
    ).toEqual({
      kind: "spec",
      nodeId: `${from}#requirement: opt-in structured run reporting`,
    });
    expect(resolveSpecLink(catalog, from, "#notes-1")).toEqual({
      kind: "spec",
      nodeId: `${from}#notes~2`,
    });
    expect(resolveSpecLink(catalog, from, "#Environment-Variables")).toEqual({
      kind: "spec",
      nodeId: `${from}#environment variables`,
    });
  });

  it("falls back to the file when the anchor is unknown", () => {
    expect(resolveSpecLink(catalog, from, "#nope")).toEqual({
      kind: "spec",
      nodeId: `${from}#e2e`,
    });
  });

  it("classifies external and out-of-tree links", () => {
    expect(resolveSpecLink(catalog, from, "https://x.dev/a.md")).toEqual({
      kind: "external",
    });
    expect(resolveSpecLink(catalog, from, "mailto:a@b.c")).toEqual({
      kind: "external",
    });
    expect(resolveSpecLink(catalog, from, "../../scripts/a%20b.ref")).toEqual({
      kind: "unresolved",
      path: "../../scripts/a b.ref",
    });
  });
});

describe("resolveSpecReference", () => {
  const from = "platform/e2e.spec.md";

  it("prefers the current folder, then the specs root, then a unique name", () => {
    expect(resolveSpecReference(catalog, from, "data-model.spec.md")).toBe(
      "platform/data-model.spec.md#data model",
    );
    expect(
      resolveSpecReference(catalog, from, "web-console/data-model.spec.md"),
    ).toBe("web-console/data-model.spec.md#console data model");
    expect(resolveSpecReference(catalog, from, "patternfly.spec.md")).toBe(
      "standards/ui/patternfly.spec.md#patternfly",
    );
  });

  it("ignores ambiguous names and text that is not a spec file", () => {
    expect(
      resolveSpecReference(
        catalog,
        "standards/ui/patternfly.spec.md",
        "data-model.spec.md",
      ),
    ).toBeNull();
    expect(resolveSpecReference(catalog, from, "make check")).toBeNull();
    expect(resolveSpecReference(catalog, from, "README.md")).toBeNull();
  });
});
