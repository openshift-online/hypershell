import { render } from "@testing-library/react";

import type { GatewayHistorySample } from "../../../domain/fleet";
import { SandSparkline } from "./sand-sparkline";

function sample(
  running: number,
  provisioning = 0,
  failed = 0,
): GatewayHistorySample {
  return { running, provisioning, failed };
}

describe("SandSparkline", () => {
  it("renders one <path> band per phase (no total outline)", () => {
    const { container } = render(
      <svg>
        <SandSparkline
          history={[sample(1), sample(2, 1), sample(3, 0, 1), sample(4)]}
          x={0}
          y={0}
          width={100}
          height={40}
        />
      </svg>,
    );
    // running / provisioning / failed bands; no amber outline (it was confused
    // with the provisioning-phase amber band).
    expect(container.querySelectorAll("path").length).toBe(3);
    expect(container.querySelectorAll("polyline").length).toBe(0);
  });

  it("renders nothing when history is empty", () => {
    const { container } = render(
      <svg>
        <SandSparkline history={[]} x={0} y={0} width={100} height={40} />
      </svg>,
    );
    expect(container.querySelectorAll("path").length).toBe(0);
  });

  it("renders nothing when history has only one sample", () => {
    const { container } = render(
      <svg>
        <SandSparkline
          history={[sample(5)]}
          x={0}
          y={0}
          width={100}
          height={40}
        />
      </svg>,
    );
    expect(container.querySelectorAll("path").length).toBe(0);
  });

  it("all-zero series renders bands with no NaN in the path data", () => {
    const { container } = render(
      <svg>
        <SandSparkline
          history={[sample(0), sample(0), sample(0)]}
          x={0}
          y={0}
          width={100}
          height={40}
        />
      </svg>,
    );
    const paths = container.querySelectorAll("path");
    expect(paths.length).toBe(3);
    for (const p of paths) {
      expect(p.getAttribute("d")).not.toContain("NaN");
    }
  });
});
