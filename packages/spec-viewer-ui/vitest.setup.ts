import { cleanup } from "@testing-library/react";
import { afterEach } from "vitest";

afterEach(() => {
  cleanup();
});

Element.prototype.scrollIntoView = function scrollIntoView() {
  // jsdom has no layout; scrolling is a no-op in tests.
};
