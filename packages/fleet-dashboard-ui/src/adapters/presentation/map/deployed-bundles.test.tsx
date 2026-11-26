import { fireEvent, render, screen } from "@testing-library/react";
import { IntlProvider } from "react-intl";
import { vi } from "vitest";

import { identiName } from "../../../domain/map/identiname";
import type { MapNode } from "../../../domain/map/model";
import type { ReleaseBundle } from "../../../domain/promotion";
import { DeployedBundles } from "./deployed-bundles";

function bundle(version: string, digest: string): ReleaseBundle {
  return { version, digest, tag: null, date: null, prs: [] };
}

// deployedFor matches on digest, so a node stub needs only that.
function runs(digest: string): MapNode {
  return { digest, version: null } as unknown as MapNode;
}

describe("DeployedBundles", () => {
  // Verifies: SDLC-VIS-07.04
  it("shows only deployed bundles, with their count, and selects on click", () => {
    const onSelect = vi.fn();
    render(
      <IntlProvider locale="en" defaultLocale="en">
        <DeployedBundles
          releaseByDigest={{
            "sha256:a": bundle("v2", "sha256:a"),
            "sha256:b": bundle("v1", "sha256:b"),
          }}
          nodes={[runs("sha256:a"), runs("sha256:a"), runs("sha256:a")]}
          selectedSeed={null}
          onSelectBundle={onSelect}
        />
      </IntlProvider>,
    );
    const group = screen.getByRole("group", {
      name: "Deployed release bundles",
    });
    const entries = group.querySelectorAll("button");
    expect(entries).toHaveLength(1);
    const entry = screen.getByRole("button", {
      name: `v2 ${identiName("sha256:a")}: 3 deployments`,
    });
    expect(entry.textContent).toContain("3");
    fireEvent.click(entry);
    expect(onSelect).toHaveBeenCalledWith("sha256:a");
  });

  it("renders nothing when no bundle is deployed", () => {
    const { container } = render(
      <IntlProvider locale="en" defaultLocale="en">
        <DeployedBundles
          releaseByDigest={{ "sha256:b": bundle("v1", "sha256:b") }}
          nodes={[]}
          selectedSeed={null}
          onSelectBundle={() => undefined}
        />
      </IntlProvider>,
    );
    expect(container.textContent).toBe("");
  });
});
