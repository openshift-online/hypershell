import { render } from "@testing-library/react";

import type { SandboxClusterCount } from "../../../domain/fleet";
import { SandboxStrip } from "./sandbox-strip";

function cl(cluster: string, count: number): SandboxClusterCount {
  return { cluster, count };
}

describe("SandboxStrip", () => {
  it("renders one bar per populated cluster, plus the track", () => {
    const { container } = render(
      <svg>
        <SandboxStrip
          clusters={[cl("a", 3), cl("b", 6)]}
          x={0}
          y={0}
          width={100}
          height={12}
        />
      </svg>,
    );
    // track + two bars
    expect(container.querySelectorAll("rect").length).toBe(3);
  });

  it("renders nothing when there are no clusters", () => {
    const { container } = render(
      <svg>
        <SandboxStrip clusters={[]} x={0} y={0} width={100} height={12} />
      </svg>,
    );
    expect(container.querySelectorAll("rect").length).toBe(0);
  });

  it("drops zero-count clusters and renders nothing when all are zero", () => {
    const { container } = render(
      <svg>
        <SandboxStrip
          clusters={[cl("a", 0), cl("b", 0)]}
          x={0}
          y={0}
          width={100}
          height={12}
        />
      </svg>,
    );
    expect(container.querySelectorAll("rect").length).toBe(0);
  });

  it("scales the busiest cluster to the full strip height", () => {
    const { container } = render(
      <svg>
        <SandboxStrip
          clusters={[cl("a", 2), cl("b", 10)]}
          x={0}
          y={0}
          width={100}
          height={12}
        />
      </svg>,
    );
    // Bars are every rect after the track; the tallest equals the strip height.
    const bars = Array.from(container.querySelectorAll("rect")).slice(1);
    const heights = bars.map((b) => Number(b.getAttribute("height")));
    expect(Math.max(...heights)).toBe(12);
  });
});
