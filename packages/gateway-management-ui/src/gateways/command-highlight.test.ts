import { readFileSync } from "node:fs";
import { join } from "node:path";

import { describe, expect, it } from "vitest";

import { type CommandPart, highlightTemplate } from "./command-highlight";

// A snippet that exercises every bash token type the connection commands use
// (comments, command names, arguments, options, quoted strings, variables,
// heredocs, and operators) so the drift guard sees the full theme palette.
const paletteSample = [
  "# comment",
  "openshell provider create \\",
  '  --config KEY="$VALUE" \\',
  "  --config OTHER=global",
  "cat > file <<'EOF'",
  "echo done | grep x && true || false; exit 0",
].join("\n");

function reconstruct(parts: CommandPart[]): string {
  return parts
    .map((part) => (part.kind === "text" ? part.value : part.marker))
    .join("");
}

function emittedClasses(parts: CommandPart[]): Set<string> {
  const classes = new Set<string>();
  for (const part of parts) {
    for (const className of part.className.split(/\s+/).filter(Boolean)) {
      if (/^hl-(?:fg|dfg)-[0-9a-f]{3,8}$/.test(className)) {
        classes.add(className);
      }
    }
  }
  return classes;
}

describe("highlightTemplate", () => {
  it("splits a marked command into colored text and field slots", async () => {
    const command = "openshell inference set --provider ZPROV --model ZMODEL";
    const parts = await highlightTemplate(command, ["ZPROV", "ZMODEL"]);

    // Highlighting never alters the underlying text: text runs plus the markers
    // reconstruct the original command exactly.
    expect(reconstruct(parts)).toBe(command);

    const fields = parts.filter((part) => part.kind === "field");
    expect(fields.map((part) => part.marker)).toEqual(["ZPROV", "ZMODEL"]);

    // Field slots inherit the argument token color so the editor matches the
    // surrounding syntax, and static text carries color through classes only.
    for (const field of fields) {
      expect(field.className).toMatch(/hl-fg-[0-9a-f]{3,8}/);
      expect(field.className).toMatch(/hl-dfg-[0-9a-f]{3,8}/);
    }
    expect(emittedClasses(parts).size).toBeGreaterThan(0);
  });

  it("emits a field slot for every marker occurrence, including repeats", async () => {
    const command =
      "openshell provider create --name ZPROV && openshell inference set --provider ZPROV";
    const parts = await highlightTemplate(command, ["ZPROV"]);

    const markers = parts
      .filter((part) => part.kind === "field")
      .map((part) => part.marker);
    expect(markers).toEqual(["ZPROV", "ZPROV"]);
  });

  it("is deterministic across calls (highlighter reused)", async () => {
    const first = await highlightTemplate("openshell inference set", []);
    const second = await highlightTemplate("openshell inference set", []);

    expect(second).toEqual(first);
  });

  it("only emits token classes the stylesheet defines", async () => {
    const parts = await highlightTemplate(paletteSample, []);

    const emitted = emittedClasses(parts);
    expect(emitted.size).toBeGreaterThan(0);

    const stylesheet = readFileSync(
      join(process.cwd(), "src/gateways/gateway-connection-steps.module.css"),
      "utf8",
    );
    const missing = [...emitted].filter(
      (className) => !stylesheet.includes(`.${className}`),
    );
    expect(missing).toEqual([]);
  });
});
