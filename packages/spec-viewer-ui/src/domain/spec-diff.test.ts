import { describe, expect, it } from "vitest";

import { markedMarkdown } from "../adapters/marked-markdown";
import {
  diffCatalogs,
  isEmptyDiff,
  type DiffEntry,
  type SpecDiff,
} from "./spec-diff";
import { buildCatalog, parseSpecFile } from "./spec-document";

function catalog(files: Record<string, string>) {
  return buildCatalog(
    Object.entries(files).map(([path, content]) =>
      parseSpecFile(path, markedMarkdown.lex(content)),
    ),
  );
}

const BASE = `# Tls

### Requirement: TLS-01 -- Certificates rotate

Certificates SHALL rotate before expiry.

#### Scenario: TLS-01.01 -- Expiring certificate

- WHEN a certificate expires within a day
- THEN it SHALL be renewed

#### Scenario: TLS-01.02 -- Revoked certificate

- WHEN a certificate is revoked
- THEN it SHALL be replaced

### Requirement: TLS-02 -- Unchanged

The platform SHALL stay the same.
`;

/** Walks every entry of the diff. */
function all(diff: SpecDiff): DiffEntry[] {
  return [...diff.byKey.values()];
}

const items = (diff: SpecDiff, status: string): string[] =>
  all(diff)
    .filter((e) => e.kind === "item" && e.status === status)
    .map((e) => e.node?.specId ?? e.node?.title ?? "");

describe("diffCatalogs", () => {
  it("is empty for equal catalogs", () => {
    const diff = diffCatalogs(
      catalog({ "p/tls.spec.md": BASE }),
      catalog({ "p/tls.spec.md": BASE }),
    );
    expect(isEmptyDiff(diff)).toBe(true);
    expect(diff.counts).toEqual({ added: 0, removed: 0, changed: 0 });
  });

  // Verifies: DEV-SPV-18.01
  it("labels items only in the current branch added, and only in the compared one removed", () => {
    const current = BASE.replace(
      "### Requirement: TLS-02",
      `### Requirement: TLS-03 -- Brand new

The platform SHALL be new.

### Requirement: TLS-02`,
    ).replace(
      /#### Scenario: TLS-01\.02[\s\S]*?(?=### Requirement: TLS-03)/u,
      "",
    );
    const diff = diffCatalogs(
      catalog({ "p/tls.spec.md": BASE }),
      catalog({ "p/tls.spec.md": current }),
    );
    expect(items(diff, "added")).toEqual(["TLS-03"]);
    expect(items(diff, "removed")).toEqual(["TLS-01.02"]);
    expect(items(diff, "changed")).toEqual([]);
    const removed = all(diff).find((e) => e.status === "removed");
    // A removed item is read from the compared branch.
    expect(removed?.side).toBe("compare");
    expect(diff.counts).toEqual({ added: 1, removed: 1, changed: 0 });
  });

  // Verifies: DEV-SPV-18.02
  it("reports a reworded item as changed, with both texts", () => {
    const current = BASE.replace("before expiry", "a week before expiry");
    const diff = diffCatalogs(
      catalog({ "p/tls.spec.md": BASE }),
      catalog({ "p/tls.spec.md": current }),
    );
    expect(items(diff, "changed")).toEqual(["TLS-01"]);
    const changed = all(diff).find((e) => e.status === "changed");
    expect(changed?.before).toContain("before expiry");
    expect(changed?.before).not.toContain("a week");
    expect(changed?.after).toContain("a week before expiry");
    expect(changed?.before?.startsWith("### Requirement: TLS-01")).toBe(true);
  });

  it("matches a renamed requirement by its ID", () => {
    const current = BASE.replace("Certificates rotate", "Certificates renew");
    const diff = diffCatalogs(
      catalog({ "p/tls.spec.md": BASE }),
      catalog({ "p/tls.spec.md": current }),
    );
    expect(items(diff, "changed")).toEqual(["TLS-01"]);
    expect(items(diff, "added")).toEqual([]);
    expect(items(diff, "removed")).toEqual([]);
  });

  it("keeps an unchanged requirement as the parent of a changed scenario", () => {
    const current = BASE.replace("within a day", "within an hour");
    const diff = diffCatalogs(
      catalog({ "p/tls.spec.md": BASE }),
      catalog({ "p/tls.spec.md": current }),
    );
    const file = diff.entries[0]?.children[0];
    expect(file?.kind).toBe("file");
    const requirement = file?.children[0];
    expect(requirement?.node?.specId).toBe("TLS-01");
    expect(requirement?.status).toBe("unchanged");
    expect(requirement?.children.map((c) => c.node?.specId)).toEqual([
      "TLS-01.01",
    ]);
    // The other requirement did not change, so it is not listed.
    expect(file?.children).toHaveLength(1);
  });

  it("lists a whole new file as added and a deleted file as removed", () => {
    const diff = diffCatalogs(
      catalog({ "p/old.spec.md": BASE }),
      catalog({ "p/new.spec.md": BASE.replaceAll("TLS", "NEW") }),
    );
    const files = all(diff).filter((e) => e.kind === "file");
    expect(files.map((f) => [f.path, f.status, f.side])).toEqual([
      ["p/new.spec.md", "added", "current"],
      ["p/old.spec.md", "removed", "compare"],
    ]);
    expect(items(diff, "removed")).toContain("TLS-01");
    expect(items(diff, "added")).toContain("NEW-01.01");
  });

  it("shows folders only above files that differ", () => {
    const diff = diffCatalogs(
      catalog({ "a/x.spec.md": BASE, "b/y.spec.md": BASE }),
      catalog({
        "a/x.spec.md": BASE,
        "b/y.spec.md": BASE.replace("expiry", "deadline"),
      }),
    );
    expect(diff.entries.map((e) => [e.kind, e.path])).toEqual([
      ["folder", "b"],
    ]);
  });

  it("matches an item without an ID by file and heading", () => {
    const base = "# Doc\n\n### Requirement: Plain\n\nIt SHALL work.\n";
    const diff = diffCatalogs(
      catalog({ "d.spec.md": base }),
      catalog({ "d.spec.md": base.replace("work", "work well") }),
    );
    expect(items(diff, "changed")).toEqual(["Plain"]);
  });

  it("does not count a moved, unchanged requirement", () => {
    const diff = diffCatalogs(
      catalog({ "a.spec.md": BASE, "b.spec.md": "# Tls\n" }),
      catalog({ "a.spec.md": "# Tls\n", "b.spec.md": BASE }),
    );
    expect(items(diff, "removed")).toEqual([]);
    expect(items(diff, "added")).toEqual([]);
  });

  it("ignores trailing-space and line-ending edits", () => {
    const diff = diffCatalogs(
      catalog({ "p/tls.spec.md": BASE }),
      catalog({ "p/tls.spec.md": BASE.replaceAll("\n", "  \n") }),
    );
    expect(isEmptyDiff(diff)).toBe(true);
  });

  it("compares plain text outside requirements", () => {
    const base = "# Doc\n\n## Background\n\nOld words.\n";
    const diff = diffCatalogs(
      catalog({ "d.spec.md": base }),
      catalog({ "d.spec.md": base.replace("Old", "New") }),
    );
    expect(items(diff, "changed")).toEqual(["Background"]);
  });
});
