import { markedMarkdown } from "../adapters/marked-markdown";
import { buildConformanceReport } from "./conformance";
import { fileKey, filterConformance, folderKey } from "./conformance-filter";
import { buildSearchIndex, searchSpecs, type SearchHit } from "./search";
import { buildCatalog, parseSpecFile } from "./spec-document";

const FEED = `# Feeding

Intro about kibble.

### Requirement: FED-ONE-01 -- Feed the quokka

The keeper SHALL feed the quokka.

#### Scenario: FED-ONE-01.01 -- Morning meal

- WHEN the sun rises
- THEN the keeper serves kibble

#### Scenario: FED-ONE-01.02 -- Evening meal

- WHEN the sun sets
- THEN the keeper serves leaves

### Requirement: FED-ONE-02 -- Water

The keeper SHALL refill water.
`;

const OTHER = `# Other

### Requirement: OTH-ONE-01 -- Unrelated

Nothing to see.
`;

const catalog = buildCatalog(
  [
    ["zoo/feed.spec.md", FEED],
    ["zoo/other.spec.md", OTHER],
  ].map(([path = "", content = ""]) =>
    parseSpecFile(path, markedMarkdown.lex(content)),
  ),
);
const report = buildConformanceReport(catalog);
const index = buildSearchIndex(catalog);

const hitsFor = (query: string): Map<string, SearchHit> =>
  new Map(searchSpecs(index, query).map((hit) => [hit.nodeId, hit]));

const id = (title: string): string => {
  for (const node of catalog.nodesById.values()) {
    if (node.title === title) return node.id;
  }
  throw new Error(`No node ${title}`);
};

describe("filterConformance", () => {
  it("is off without a search or star filter", () => {
    expect(filterConformance(report.root, {})).toBeNull();
  });

  it("shows matching scenarios and expands the rows above them", () => {
    const view = filterConformance(report.root, { hits: hitsFor("leaves") });
    expect(view).not.toBeNull();
    expect([...(view?.visible ?? [])].sort()).toEqual(
      [
        folderKey("zoo"),
        fileKey("zoo/feed.spec.md"),
        id("Feed the quokka"),
        id("Evening meal"),
      ].sort(),
    );
    expect([...(view?.expanded ?? [])].sort()).toEqual(
      [
        folderKey("zoo"),
        fileKey("zoo/feed.spec.md"),
        id("Feed the quokka"),
      ].sort(),
    );
  });

  it("keeps a requirement that matched by itself collapsed", () => {
    const view = filterConformance(report.root, { hits: hitsFor("refill") });
    expect(view?.visible.has(id("Water"))).toBe(true);
    expect(view?.expanded.has(id("Water"))).toBe(false);
    expect(view?.visible.has(fileKey("zoo/other.spec.md"))).toBe(false);
  });

  it("stands the file row in for text outside requirements", () => {
    const view = filterConformance(report.root, { hits: hitsFor("intro") });
    expect(view?.visible.has(fileKey("zoo/feed.spec.md"))).toBe(true);
    expect(view?.expanded.has(fileKey("zoo/feed.spec.md"))).toBe(false);
    expect(view?.textHits.get("zoo/feed.spec.md")).toHaveLength(1);
  });

  it("shows starred items, with all scenarios of a starred requirement", () => {
    const view = filterConformance(report.root, {
      starred: new Set([id("Feed the quokka"), id("Unrelated")]),
    });
    expect(view?.visible.has(id("Morning meal"))).toBe(true);
    expect(view?.visible.has(id("Evening meal"))).toBe(true);
    expect(view?.visible.has(id("Water"))).toBe(false);
    expect(view?.visible.has(fileKey("zoo/other.spec.md"))).toBe(true);
  });

  it("combines the search with the star filter", () => {
    const view = filterConformance(report.root, {
      hits: hitsFor("keeper"),
      starred: new Set([id("Morning meal")]),
    });
    expect(view?.visible.has(id("Morning meal"))).toBe(true);
    expect(view?.visible.has(id("Evening meal"))).toBe(false);
    // The requirement shows as the scenario's parent, not as a match.
    expect(view?.visible.has(id("Feed the quokka"))).toBe(true);
    expect(view?.visible.has(id("Water"))).toBe(false);
  });
});
