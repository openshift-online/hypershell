import { cleanup, configure } from "@testing-library/react";
import { afterEach } from "vitest";

// Each App test first waits for the spec catalog to load and parse. On
// shared CI runners that can take over the default 1 s wait.
configure({ asyncUtilTimeout: 5000 });

afterEach(() => {
  cleanup();
});

Element.prototype.scrollIntoView = function scrollIntoView() {
  // jsdom has no layout; scrolling is a no-op in tests.
};
