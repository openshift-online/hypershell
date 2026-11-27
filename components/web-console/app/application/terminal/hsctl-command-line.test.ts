import { describe, expect, it } from "vitest";

import {
  analyzeCommandLine,
  hsctlTopLevelCommands,
  splitCommandLine,
} from "./hsctl-command-line";

describe("splitCommandLine", () => {
  // Verifies: WEB-TERM-04
  it("splits words with single quotes, double quotes, and backslash escapes", () => {
    expect(
      splitCommandLine(
        `list gateways --search "name like 'prod%'" a\\ b 'it''s' "x\\"y" ""`,
      ),
    ).toEqual({
      kind: "words",
      words: [
        "list",
        "gateways",
        "--search",
        "name like 'prod%'",
        "a b",
        "its",
        'x"y',
        "",
      ],
    });
  });

  // Verifies: WEB-TERM-04
  it("keeps backslashes inside single quotes and before ordinary characters in double quotes", () => {
    expect(splitCommandLine(`'a\\b' "c\\d" "\\$" \\|`)).toEqual({
      kind: "words",
      words: ["a\\b", "c\\d", "$", "|"],
    });
  });

  // Verifies: WEB-TERM-04
  it("collapses whitespace and returns no words for a blank line", () => {
    expect(splitCommandLine("  \t ")).toEqual({ kind: "words", words: [] });
    expect(splitCommandLine(" get   gateway\tg1 ")).toEqual({
      kind: "words",
      words: ["get", "gateway", "g1"],
    });
  });

  // Verifies: WEB-TERM-04.02
  it.each([
    "hsctl list gateways | grep prod",
    "list gateways > out.txt",
    "list gateways < in.txt",
    "list gateways; rm -rf /",
    "list gateways &",
    "get gateway $(whoami)",
    "get gateway `whoami`",
    'get gateway "$(whoami)"',
    'get gateway "`whoami`"',
  ])("refuses unquoted shell operators in %s", (line) => {
    expect(splitCommandLine(line)).toEqual({ kind: "shell-operator" });
  });

  // Verifies: WEB-TERM-04
  it("treats quoted or escaped operators as text", () => {
    expect(splitCommandLine(`get gateway '|;&<>' "a|b" \\; '$(x)'`)).toEqual({
      kind: "words",
      words: ["get", "gateway", "|;&<>", "a|b", ";", "$(x)"],
    });
  });

  // Verifies: WEB-TERM-04
  it("reports an unterminated quote or trailing backslash as incomplete", () => {
    expect(splitCommandLine(`get 'gateway`)).toEqual({ kind: "incomplete" });
    expect(splitCommandLine(`get "gateway`)).toEqual({ kind: "incomplete" });
    expect(splitCommandLine(`get gateway\\`)).toEqual({ kind: "incomplete" });
  });
});

describe("analyzeCommandLine", () => {
  // Verifies: WEB-TERM-04.01
  it("drops an optional leading hsctl word", () => {
    expect(analyzeCommandLine("hsctl list gateways")).toEqual({
      argv: ["list", "gateways"],
      kind: "run",
    });
    expect(analyzeCommandLine("list gateways")).toEqual({
      argv: ["list", "gateways"],
      kind: "run",
    });
    expect(analyzeCommandLine("hsctl")).toEqual({ argv: [], kind: "run" });
    expect(analyzeCommandLine("hsctl --help")).toEqual({
      argv: ["--help"],
      kind: "run",
    });
  });

  // Verifies: WEB-TERM-04
  it("accepts every top-level hsctl command without the hsctl word", () => {
    for (const command of hsctlTopLevelCommands) {
      expect(analyzeCommandLine(command)).toEqual({
        argv: [command],
        kind: "run",
      });
    }
  });

  // Verifies: WEB-TERM-04
  it("names other programs, which must run in a local terminal", () => {
    expect(analyzeCommandLine("kubectl get pods")).toEqual({
      kind: "not-hsctl",
      program: "kubectl",
    });
    expect(analyzeCommandLine("openshell gateway add x")).toEqual({
      kind: "not-hsctl",
      program: "openshell",
    });
  });

  // Verifies: WEB-TERM-04
  it("recognizes clear, empty lines, refusals, and incomplete lines", () => {
    expect(analyzeCommandLine("clear")).toEqual({ kind: "clear" });
    expect(analyzeCommandLine("clear now")).toEqual({
      kind: "not-hsctl",
      program: "clear",
    });
    expect(analyzeCommandLine("   ")).toEqual({ kind: "empty" });
    expect(analyzeCommandLine("kubectl get pods | wc")).toEqual({
      kind: "shell-operator",
    });
    expect(analyzeCommandLine("get 'x")).toEqual({ kind: "incomplete" });
  });
});
