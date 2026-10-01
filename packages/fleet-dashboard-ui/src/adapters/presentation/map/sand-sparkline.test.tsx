import { render } from "@testing-library/react";
import { SandSparkline } from "./sand-sparkline";

describe("SandSparkline", () => {
  it("renders one <polygon> and one <polyline> for a normal series", () => {
    const { container } = render(
      <svg>
        <SandSparkline
          history={[1, 2, 3, 4]}
          x={0}
          y={0}
          width={100}
          height={40}
        />
      </svg>,
    );
    expect(container.querySelectorAll("polygon").length).toBe(1);
    expect(container.querySelectorAll("polyline").length).toBe(1);
  });

  it("renders nothing when history is empty", () => {
    const { container } = render(
      <svg>
        <SandSparkline history={[]} x={0} y={0} width={100} height={40} />
      </svg>,
    );
    expect(container.querySelectorAll("polygon").length).toBe(0);
    expect(container.querySelectorAll("polyline").length).toBe(0);
  });

  it("renders nothing when history has only one sample", () => {
    const { container } = render(
      <svg>
        <SandSparkline history={[5]} x={0} y={0} width={100} height={40} />
      </svg>,
    );
    expect(container.querySelectorAll("polygon").length).toBe(0);
    expect(container.querySelectorAll("polyline").length).toBe(0);
  });

  it("flat series still renders a polygon with no NaN in points", () => {
    const { container } = render(
      <svg>
        <SandSparkline
          history={[3, 3, 3]}
          x={0}
          y={0}
          width={100}
          height={40}
        />
      </svg>,
    );
    const polygon = container.querySelector("polygon");
    expect(polygon).not.toBeNull();
    expect(polygon?.getAttribute("points")).not.toContain("NaN");
  });
});
