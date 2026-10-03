import { render } from "@testing-library/react";

import { SandboxSparkline } from "./sandbox-sparkline";

describe("SandboxSparkline", () => {
  it("renders a single teal band path", () => {
    const { container } = render(
      <svg>
        <SandboxSparkline
          history={[1, 2, 3, 4]}
          x={0}
          y={0}
          width={100}
          height={40}
        />
      </svg>,
    );
    // One band (unlike the gateway sparkline's three stacked phases).
    expect(container.querySelectorAll("path").length).toBe(1);
    expect(container.querySelectorAll("polyline").length).toBe(0);
  });

  it("renders nothing when history is empty", () => {
    const { container } = render(
      <svg>
        <SandboxSparkline history={[]} x={0} y={0} width={100} height={40} />
      </svg>,
    );
    expect(container.querySelectorAll("path").length).toBe(0);
  });

  it("renders nothing when history has only one sample", () => {
    const { container } = render(
      <svg>
        <SandboxSparkline history={[5]} x={0} y={0} width={100} height={40} />
      </svg>,
    );
    expect(container.querySelectorAll("path").length).toBe(0);
  });

  it("all-zero series renders a band with no NaN in the path data", () => {
    const { container } = render(
      <svg>
        <SandboxSparkline
          history={[0, 0, 0]}
          x={0}
          y={0}
          width={100}
          height={40}
        />
      </svg>,
    );
    const paths = container.querySelectorAll("path");
    expect(paths.length).toBe(1);
    for (const p of paths) {
      expect(p.getAttribute("d")).not.toContain("NaN");
    }
  });
});
