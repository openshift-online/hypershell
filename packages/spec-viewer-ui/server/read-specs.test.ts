import { mkdir, mkdtemp, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import path from "node:path";

import { isSpecFile, readSpecFiles } from "./read-specs";

describe("readSpecFiles", () => {
  let root: string;

  beforeEach(async () => {
    root = await mkdtemp(path.join(tmpdir(), "spec-viewer-"));
    await mkdir(path.join(root, "platform"));
    await mkdir(path.join(root, "web", "storybook-static"), {
      recursive: true,
    });
    await writeFile(path.join(root, "platform", "b.spec.md"), "# B\n");
    await writeFile(path.join(root, "a.md"), "# A\n");
    await writeFile(path.join(root, "platform", "c.tsx"), "export {};\n");
    await writeFile(path.join(root, "web", "storybook-static", "x.md"), "#\n");
  });

  afterEach(async () => {
    await rm(root, { recursive: true, force: true });
  });

  it("reads markdown files with posix relative paths, skipping build output", async () => {
    expect(await readSpecFiles(root)).toEqual([
      { path: "a.md", content: "# A\n" },
      { path: "platform/b.spec.md", content: "# B\n" },
    ]);
  });

  it("recognizes markdown files inside the root only", () => {
    expect(isSpecFile(root, path.join(root, "platform", "b.spec.md"))).toBe(
      true,
    );
    expect(isSpecFile(root, path.join(root, "platform", "c.tsx"))).toBe(false);
    expect(isSpecFile(root, path.join(root, "..", "other.md"))).toBe(false);
  });
});
