import { execFile } from "node:child_process";
import { mkdir, mkdtemp, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import path from "node:path";
import { promisify } from "node:util";

import { specFilesFromTree } from "./read-specs";
import {
  createRepository,
  findMarkers,
  findReferences,
  idPattern,
  isMarkerCandidate,
  isTestFile,
  markerLines,
  readText,
  repositoryRoot,
  UnknownRefError,
  WORKING_TREE,
} from "./repository";

const run = promisify(execFile);

describe("idPattern", () => {
  it("matches the ID as a whole identifier", () => {
    const pattern = idPattern("QRX-01");
    expect(pattern.test("// QRX-01 -- provision time")).toBe(true);
    expect(pattern.test('t.Run("QRX-01", ...)')).toBe(true);
    expect(pattern.test("(QRX-01).")).toBe(true);
    expect(pattern.test("QRX-010")).toBe(false);
    expect(pattern.test("XQRX-01")).toBe(false);
    expect(pattern.test("QRX-01-S1")).toBe(false);
    expect(pattern.test("QRX-01.2")).toBe(false);
    expect(idPattern("QRX-01-S1").test("covers QRX-01-S1")).toBe(true);
  });
});

describe("isTestFile", () => {
  it.each([
    ["components/control-plane/internal/x_test.go", true],
    ["components/web-console/app/a.test.tsx", true],
    ["components/web-console/e2e/login.spec.ts", true],
    ["tests/e2e/suite.go", true],
    ["scripts/kind/up_test.sh", true],
    ["components/control-plane/internal/x.go", false],
    ["specs/platform/e2e-testing.spec.md", false],
    ["tests/e2e/README.md", false],
  ])("classifies %s", (file, expected) => {
    expect(isTestFile(file)).toBe(expected);
  });
});

describe("createRepository", () => {
  let root: string;

  beforeEach(async () => {
    root = await mkdtemp(path.join(tmpdir(), "spec-viewer-repo-"));
    await run("git", ["init", "-q"], { cwd: root });
    await mkdir(path.join(root, "pkg"));
    await writeFile(path.join(root, ".gitignore"), "ignored.go\n");
    await writeFile(
      path.join(root, "pkg", "rotate_test.go"),
      "package pkg\n\n// QRX-01-S1\nfunc TestRotate() { /* QRX-01 */ }\n",
    );
    await writeFile(path.join(root, "README.md"), "See QRX-01.\nQRX-010\n");
    await writeFile(path.join(root, "ignored.go"), "// QRX-01\n");
    await writeFile(path.join(root, "blob.bin"), Buffer.from([0, 1, 2, 71]));
  });

  afterEach(async () => {
    await rm(root, { recursive: true, force: true });
  });

  it("finds whole-ID mentions in listed text files, tests first", async () => {
    const repository = createRepository(await repositoryRoot(root));
    const tree = await repository.tree(WORKING_TREE);
    expect(await findReferences(tree, "QRX-01")).toEqual([
      {
        path: "pkg/rotate_test.go",
        isTest: true,
        matches: [{ line: 4, text: "func TestRotate() { /* QRX-01 */ }" }],
      },
      {
        path: "README.md",
        isTest: false,
        matches: [{ line: 1, text: "See QRX-01." }],
      },
    ]);
  });

  it("finds marker lines outside markdown and lockfiles", async () => {
    await writeFile(
      path.join(root, "pkg", "feed_test.go"),
      "package pkg\n\n// Verifies: ABC-DEF-01.01\nfunc TestFeed() {}\n",
    );
    await writeFile(path.join(root, "notes.md"), "// Verifies: ABC-DEF-01\n");
    await writeFile(path.join(root, "go.sum"), "// Verifies: ABC-DEF-01\n");
    const repository = createRepository(root);
    expect(await findMarkers(await repository.tree(WORKING_TREE))).toEqual([
      {
        path: "pkg/feed_test.go",
        line: 3,
        text: "// Verifies: ABC-DEF-01.01",
        following: ["func TestFeed() {}"],
      },
    ]);
  });

  it("reads only files git lists, as text", async () => {
    const tree = await createRepository(root).tree(WORKING_TREE);
    expect(await readText(tree, "README.md")).toBe("See QRX-01.\nQRX-010\n");
    expect(await readText(tree, "ignored.go")).toBeNull();
    expect(await readText(tree, "../etc/passwd")).toBeNull();
    expect(await readText(tree, "blob.bin")).toBeNull();
  });

  it("reads a branch from git, apart from uncommitted edits", async () => {
    const git = (...args: string[]) => run("git", args, { cwd: root });
    await mkdir(path.join(root, "specs", "node_modules"), { recursive: true });
    await writeFile(path.join(root, "specs", "a.md"), "committed\n");
    await writeFile(path.join(root, "specs", "node_modules", "x.md"), "x\n");
    await git("add", "-A");
    await git(
      "-c",
      "user.name=t",
      "-c",
      "user.email=t@example.com",
      "commit",
      "-qm",
      "base",
    );
    await git("branch", "feature/x");
    await writeFile(path.join(root, "specs", "a.md"), "edited\n");
    await writeFile(path.join(root, "specs", "b.md"), "new\n");

    const repository = createRepository(root);
    const refs = await repository.listRefs();
    expect(refs.default).toBe(WORKING_TREE);
    expect(refs.refs).toEqual(
      expect.arrayContaining([WORKING_TREE, "feature/x"]),
    );

    const branch = await repository.tree("feature/x");
    expect(await specFilesFromTree(branch, "specs/")).toEqual([
      { path: "a.md", content: "committed\n" },
    ]);
    expect(await readText(branch, "blob.bin")).toBeNull();
    expect(await readText(branch, "pkg/rotate_test.go")).toContain("QRX-01");
    expect(
      await specFilesFromTree(await repository.tree(WORKING_TREE), "specs/"),
    ).toEqual([
      { path: "a.md", content: "edited\n" },
      { path: "b.md", content: "new\n" },
    ]);
    // The same commit is read once.
    expect(await repository.tree("feature/x")).toBe(branch);
    await expect(repository.tree("--output=x")).rejects.toThrow(
      UnknownRefError,
    );
  });
});

describe("markerLines", () => {
  it("keeps the lines up to the first code line", () => {
    expect(
      markerLines(
        "a.test.ts",
        "x\n  # Verifies: ABC-DEF-01\n\n// note\nit('works')\nmore\n",
      ),
    ).toEqual([
      {
        path: "a.test.ts",
        line: 2,
        text: "  # Verifies: ABC-DEF-01",
        following: ["", "// note", "it('works')"],
      },
    ]);
    expect(markerLines("a.go", "no markers here")).toEqual([]);
  });

  it("skips markdown, lockfiles, and agent locks", () => {
    expect(isMarkerCandidate("pkg/a_test.go")).toBe(true);
    expect(isMarkerCandidate("specs/a.md")).toBe(false);
    expect(isMarkerCandidate("web/pnpm-lock.yaml")).toBe(false);
    expect(isMarkerCandidate("components/agents/locks/x.json")).toBe(false);
  });
});
