import { markedMarkdown } from "../adapters/marked-markdown";
import {
  buildSearchIndex,
  highlightTerms,
  parseQuery,
  queryTerms,
  searchSpecs,
} from "./search";
import {
  buildCatalog,
  classifyHeading,
  fileEntryId,
  parseSpecFile,
  type SpecCatalog,
} from "./spec-document";
import { buildSpecTree } from "./spec-tree";

const GATEWAY_SPEC = `# Gateway Provision Time

Intro text about provisioning dashboards.

## Requirements

### Requirement: QRX-01 -- Provision Duration Contract

The platform SHALL compute durations in seconds.

#### Scenario: Histogram present

- GIVEN observations exist
- THEN mean SHALL be computable

##### Notes

Bucket boundaries are explicit.

### Requirement ZZ-A11Y-01: Keyboard access

All controls SHALL be reachable.

## Non-Goals

Per-gateway breakdown is out of scope.
`;

const SECURITY_SPEC = `# Security Standards

## Secret Handling

Secrets are never logged.
The rotation of the database password is weekly.

#### Scenario: Store access respects direction

- GIVEN a writer
`;

const RUNTIME_SPEC = `# Agent runtime images

## ARI-01 -- Two runtime profiles

Profiles are minimal and full.

\`\`\`text
# not a heading
\`\`\`

## Purpose

Purpose text
spans lines.
`;

function catalog(): SpecCatalog {
  return buildCatalog(
    [
      ["platform/gateway.spec.md", GATEWAY_SPEC],
      ["standards/security.spec.md", SECURITY_SPEC],
      ["platform/runtime.spec.md", RUNTIME_SPEC],
    ].map(([path = "", content = ""]) =>
      parseSpecFile(path, markedMarkdown.lex(content)),
    ),
  );
}

describe("classifyHeading", () => {
  it.each([
    ["Requirement: QRX-00 -- Histogram", "spec", "Histogram", "QRX-00"],
    ["Requirement ZZ-A11Y-01: Keyboard", "spec", "Keyboard", "ZZ-A11Y-01"],
    [
      "Requirement: Service Account Support",
      "spec",
      "Service Account Support",
      undefined,
    ],
    ["ARI-02 -- Pinned inputs", "spec", "Pinned inputs", "ARI-02"],
    ["Scenario: Zero observations", "scenario", "Zero observations", undefined],
    ["Scenario: QRX-01-S2 -- Zero data", "scenario", "Zero data", "QRX-01-S2"],
    ["Scenario: QRX-01.3: Dotted", "scenario", "Dotted", "QRX-01.3"],
    ["Scenario: HTTP-2 upgrade", "scenario", "HTTP-2 upgrade", undefined],
    ["Requirements", "nospec", "Requirements", undefined],
    [
      "Requirement and Scenario IDs",
      "nospec",
      "Requirement and Scenario IDs",
      undefined,
    ],
    [
      "Requirement: SEC-RBAC-01 -- Authority",
      "spec",
      "Authority",
      "SEC-RBAC-01",
    ],
    [
      "Scenario: SEC-RBAC-01.02 -- Revokes",
      "scenario",
      "Revokes",
      "SEC-RBAC-01.02",
    ],
    ["`TLS` **settings**", "nospec", "TLS settings", undefined],
  ])("classifies %s", (text, kind, title, specId) => {
    const result = classifyHeading(text, 3);
    expect(result.kind).toBe(kind);
    expect(result.title).toBe(title);
    expect(result.specId).toBe(specId);
  });

  it("does not treat an ID-like file title as a spec", () => {
    expect(classifyHeading("ARI-01 Overview", 1).kind).toBe("nospec");
  });
});

describe("parseSpecFile", () => {
  it("extracts the file title, specs, scenarios, and their parents", () => {
    const file = catalog().filesByPath.get("platform/gateway.spec.md");
    expect(file?.title).toBe("Gateway Provision Time");
    const summary = file?.nodes.map((node) => [
      node.kind,
      node.title,
      node.parentId,
    ]);
    expect(summary).toEqual([
      ["nospec", "Gateway Provision Time", null],
      ["nospec", "Requirements", null],
      ["spec", "Provision Duration Contract", null],
      [
        "scenario",
        "Histogram present",
        "platform/gateway.spec.md#requirement: qrx-01 -- provision duration contract",
      ],
      ["nospec", "Notes", null],
      ["spec", "Keyboard access", null],
      ["nospec", "Non-Goals", null],
    ]);
  });

  it("assigns text under nested headings to the enclosing scenario", () => {
    const file = catalog().filesByPath.get("platform/gateway.spec.md");
    const notes = file?.sections.find((section) => section.heading === "Notes");
    expect(notes?.ownerId).toBe(
      "platform/gateway.spec.md#scenario: histogram present",
    );
  });

  it("records source lines and ignores headings inside code blocks", () => {
    const file = catalog().filesByPath.get("platform/runtime.spec.md");
    expect(file?.nodes.map((node) => [node.title, node.line])).toEqual([
      ["Agent runtime images", 1],
      ["Two runtime profiles", 3],
      ["Purpose", 11],
    ]);
  });

  it("parents an orphan scenario to its nearest heading", () => {
    const file = catalog().filesByPath.get("standards/security.spec.md");
    const scenario = file?.nodes.find((node) => node.kind === "scenario");
    expect(scenario?.parentId).toBe(
      "standards/security.spec.md#secret handling",
    );
  });

  it("keeps preamble text before the first heading and dedupes keys", () => {
    const file = parseSpecFile(
      "notes.md",
      markedMarkdown.lex("Loose text.\n\n## Same\n\na\n\n## Same\n\nb\n"),
    );
    expect(file.title).toBe("notes.md");
    expect(file.nodes.map((node) => node.id)).toEqual([
      "notes.md#",
      "notes.md#same",
      "notes.md#same~2",
    ]);
    expect(fileEntryId(file)).toBe("notes.md#");
  });
});

describe("searchSpecs", () => {
  const index = buildSearchIndex(catalog());
  const titles = (query: string) => {
    const nodes = catalog().nodesById;
    return searchSpecs(index, query).map((hit) => nodes.get(hit.nodeId)?.title);
  };

  it("returns nothing for an empty query", () => {
    expect(searchSpecs(index, "   ")).toEqual([]);
  });

  it("ranks heading matches above body matches", () => {
    expect(titles("histogram")[0]).toBe("Histogram present");
  });

  it("reports body text under a nested heading against its scenario", () => {
    expect(titles("bucket boundaries")).toEqual(["Histogram present"]);
  });

  it("reports text outside any spec against its nearest heading", () => {
    expect(titles("per-gateway")).toEqual(["Non-Goals"]);
    expect(titles("intro text")).toEqual(["Gateway Provision Time"]);
  });

  it("tolerates typos and abbreviations", () => {
    expect(titles("keybord")).toContain("Keyboard access");
    expect(titles("prvdur")).toContain("Provision Duration Contract");
  });

  it("requires every term to match", () => {
    expect(titles("keyboard histogram")).toEqual([]);
  });

  it("builds a highlighted snippet for body matches", () => {
    const [hit] = searchSpecs(index, "logged");
    expect(hit?.snippet?.text).toBe(
      "Secrets are never logged. The rotation of the database password is weekly.",
    );
    expect(hit?.snippet?.highlights).toEqual([[18, 24]]);
  });
});

describe("parseQuery", () => {
  it("splits words, quoted phrases, and bracketed same-line groups", () => {
    expect(
      parseQuery('Keycloak "Client  Secret" [database   rotation] tls]'),
    ).toEqual([
      { kind: "word", term: "keycloak" },
      { kind: "phrase", text: "client secret" },
      { kind: "line", words: ["database", "rotation"] },
      { kind: "word", term: "tls" },
    ]);
  });

  it("closes an unterminated quote or bracket at the end", () => {
    expect(parseQuery('a"b c')).toEqual([
      { kind: "word", term: "a" },
      { kind: "phrase", text: "b c" },
    ]);
    expect(parseQuery("[x y")).toEqual([{ kind: "line", words: ["x", "y"] }]);
    expect(parseQuery('"" [] ]')).toEqual([]);
  });
});

describe("searchSpecs query syntax", () => {
  const specs = catalog();
  const index = buildSearchIndex(specs);
  const titles = (query: string) =>
    searchSpecs(index, query).map(
      (hit) => specs.nodesById.get(hit.nodeId)?.title,
    );

  it("matches words spread over a spec and its scenarios", () => {
    expect(titles("seconds observations")).toEqual([
      "Provision Duration Contract",
    ]);
  });

  it("reports only the scenario when it matches on its own", () => {
    expect(titles("observations computable")).toEqual(["Histogram present"]);
  });

  it("does not spread words over separate specs", () => {
    expect(titles("seconds reachable")).toEqual([]);
  });

  it("matches quoted text literally, across line breaks", () => {
    expect(titles('"compute durations"')).toEqual([
      "Provision Duration Contract",
    ]);
    expect(titles('"durations compute"')).toEqual([]);
    expect(titles('"compute duratoins"')).toEqual([]);
    expect(titles('"text spans"')).toEqual(["Purpose"]);
  });

  it("matches bracketed words on one line in any order", () => {
    expect(titles("[database rotation]")).toEqual(["Secret Handling"]);
    expect(titles("[rotation database]")).toEqual(["Secret Handling"]);
    expect(titles("[logged rotation]")).toEqual([]);
    expect(titles("[mean computable]")).toEqual(["Histogram present"]);
    expect(titles("[seconds mean]")).toEqual([]);
  });

  it("anchors the snippet on the matching line", () => {
    const [hit] = searchSpecs(index, "[weekly database]");
    expect(hit?.snippet?.text).toBe(
      "...The rotation of the database password is weekly.",
    );
    expect(hit?.snippet?.highlights).toEqual([
      [23, 31],
      [44, 50],
    ]);
  });

  it("lists the text to highlight, including typo-corrected words", () => {
    expect(
      queryTerms('keybord "all controls" [shall reachable] shall'),
    ).toEqual(["keybord", "all controls", "shall", "reachable"]);
    const query = "keybord";
    expect(highlightTerms(query, searchSpecs(index, query))).toEqual([
      "keybord",
      "keyboard",
    ]);
  });

  it("combines clause kinds", () => {
    expect(titles('histogram "mean shall" [observations given]')).toEqual([
      "Histogram present",
    ]);
  });
});

describe("buildSpecTree", () => {
  it("groups entries into files, specs, and scenarios", () => {
    const specs = catalog();
    const { files, missing } = buildSpecTree(specs, [
      {
        nodeId:
          "standards/security.spec.md#scenario: store access respects direction",
      },
      { nodeId: "platform/gateway.spec.md#scenario: histogram present" },
      { nodeId: "platform/gateway.spec.md#non-goals" },
      { nodeId: "gone.md#x" },
    ]);
    expect(missing).toEqual(["gone.md#x"]);
    expect(
      files.map((file) => [
        file.file.path,
        file.children.map((child) => [
          child.node.title,
          child.isEntry,
          child.children.map((grand) => grand.node.title),
        ]),
      ]),
    ).toEqual([
      [
        "standards/security.spec.md",
        [["Secret Handling", false, ["Store access respects direction"]]],
      ],
      [
        "platform/gateway.spec.md",
        [
          ["Provision Duration Contract", false, ["Histogram present"]],
          ["Non-Goals", true, []],
        ],
      ],
    ]);
  });
});
