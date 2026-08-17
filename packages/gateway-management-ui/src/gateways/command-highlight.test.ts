import { describe, expect, it } from "vitest";

import { highlightCommand } from "./command-highlight";

describe("highlightCommand", () => {
  it("renders a command as dual-theme Shiki markup", async () => {
    const html = await highlightCommand("openshell gateway add --name my-gw");

    // Shiki wraps the code in a themed <pre>, preserving the original text.
    expect(html).toContain('class="shiki');
    expect(html).toContain("openshell");
    expect(html).toContain("gateway");
    // Dual-theme output exposes the dark palette via CSS variables.
    expect(html).toContain("--shiki-dark");
  });

  it("reuses the highlighter across calls", async () => {
    const first = await highlightCommand("openshell inference set");
    const second = await highlightCommand("openshell inference set");

    expect(second).toBe(first);
  });
});
