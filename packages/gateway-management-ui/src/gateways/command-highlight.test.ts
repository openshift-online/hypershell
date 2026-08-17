import { readFileSync } from "node:fs";
import { join } from "node:path";

import { describe, expect, it } from "vitest";

import { highlightCommand } from "./command-highlight";

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

describe("highlightCommand", () => {
  it("renders CSP-safe class-based Shiki markup", async () => {
    const html = await highlightCommand("openshell gateway add --name my-gw");

    // Shiki wraps the code in a themed <pre>, preserving the original text.
    expect(html).toContain('class="shiki');
    expect(html).toContain("openshell");
    expect(html).toContain("gateway");
    // The hardened CSP (style-src-attr 'none') strips inline style attributes,
    // so tokens must carry color through classes, never inline styles.
    expect(html).not.toContain("style=");
    expect(html).toMatch(/hl-fg-[0-9a-f]{3,8}/);
    expect(html).toMatch(/hl-dfg-[0-9a-f]{3,8}/);
  });

  it("reuses the highlighter across calls", async () => {
    const first = await highlightCommand("openshell inference set");
    const second = await highlightCommand("openshell inference set");

    expect(second).toBe(first);
  });

  it("only emits token classes the stylesheet defines", async () => {
    const html = await highlightCommand(paletteSample);

    const emitted = new Set(
      [...html.matchAll(/hl-(?:fg|dfg)-[0-9a-f]{3,8}/g)].map(
        (match) => match[0],
      ),
    );
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
