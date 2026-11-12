import { orderRefs } from "./ref-order";

describe("orderRefs", () => {
  it("lists main right after the default", () => {
    expect(orderRefs("@", ["@", "a", "b", "main", "z"])).toEqual([
      "@",
      "main",
      "a",
      "b",
      "z",
    ]);
  });

  it("keeps main first when it is the default", () => {
    expect(orderRefs("main", ["a", "main", "z"])).toEqual(["main", "a", "z"]);
  });

  it("does not invent a main branch", () => {
    expect(orderRefs("@", ["@", "a"])).toEqual(["@", "a"]);
  });
});
