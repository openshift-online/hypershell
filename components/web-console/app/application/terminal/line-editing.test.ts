import { describe, expect, it } from "vitest";

import {
  deleteToLineEnd,
  deleteToLineStart,
  deleteWordBefore,
  initialCommandRecall,
  recallCommand,
} from "./line-editing";

describe("line editing", () => {
  // Verifies: WEB-TERM-05
  it("deletes to the start or end of the line from the cursor", () => {
    expect(deleteToLineStart({ cursor: 5, value: "list gateways" })).toEqual({
      cursor: 0,
      value: "gateways",
    });
    expect(deleteToLineEnd({ cursor: 4, value: "list gateways" })).toEqual({
      cursor: 4,
      value: "list",
    });
    expect(deleteToLineEnd({ cursor: 99, value: "ab" })).toEqual({
      cursor: 2,
      value: "ab",
    });
  });

  // Verifies: WEB-TERM-05
  it("deletes the word before the cursor with trailing whitespace", () => {
    expect(deleteWordBefore({ cursor: 14, value: "list gateways " })).toEqual({
      cursor: 5,
      value: "list ",
    });
    expect(deleteWordBefore({ cursor: 4, value: "list gateways" })).toEqual({
      cursor: 0,
      value: " gateways",
    });
    expect(deleteWordBefore({ cursor: 0, value: "x" })).toEqual({
      cursor: 0,
      value: "x",
    });
  });

  // Verifies: WEB-TERM-05
  it("recalls earlier and later commands and restores the draft", () => {
    const commands = ["one", "two"];
    const up1 = recallCommand(commands, initialCommandRecall, "draft", "older");
    expect(up1?.value).toBe("two");
    if (up1 === null) {
      return;
    }
    const up2 = recallCommand(commands, up1.state, "two", "older");
    expect(up2?.value).toBe("one");
    if (up2 === null) {
      return;
    }
    expect(recallCommand(commands, up2.state, "one", "older")).toBeNull();
    const down1 = recallCommand(commands, up2.state, "one", "newer");
    expect(down1?.value).toBe("two");
    if (down1 === null) {
      return;
    }
    expect(recallCommand(commands, down1.state, "two", "newer")).toEqual({
      state: initialCommandRecall,
      value: "draft",
    });
    expect(
      recallCommand(commands, initialCommandRecall, "x", "newer"),
    ).toBeNull();
    expect(recallCommand([], initialCommandRecall, "x", "older")).toBeNull();
  });
});
