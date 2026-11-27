import { describe, expect, it } from "vitest";

import { currentPathAndQuery, signInHref } from "./sign-in";

describe("signInHref", () => {
  // Verifies: WEB-UDOC-03.01
  it("encodes the docs path as return_to", () => {
    expect(signInHref("/docs/runbooks/github-jira")).toBe(
      "/auth/login?return_to=%2Fdocs%2Frunbooks%2Fgithub-jira",
    );
  });

  it("keeps the query in return_to", () => {
    expect(
      signInHref(
        currentPathAndQuery({
          pathname: "/docs/refusals/resource-limits",
          search: "?policy=sandbox-cpu-limit",
        }),
      ),
    ).toBe(
      "/auth/login?return_to=%2Fdocs%2Frefusals%2Fresource-limits%3Fpolicy%3Dsandbox-cpu-limit",
    );
  });
});
