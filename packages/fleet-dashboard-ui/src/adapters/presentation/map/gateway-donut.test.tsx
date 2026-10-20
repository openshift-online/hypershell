import { render } from "@testing-library/react";
import { GatewayDonut } from "./gateway-donut";

/** A segment is VISIBLE when its dash array's drawn length (first value) is non-zero.
 *  All segments are now always rendered (zero-length when empty) so CSS can transition
 *  them smoothly, so visibility is read from the dash array, not the circle count. */
function visibleSegmentCount(container: HTMLElement): number {
  const rotateGroup = container.querySelector("g[transform]");
  const circles = Array.from(
    rotateGroup?.querySelectorAll("circle") ??
      container.querySelectorAll("circle"),
  );
  return circles.filter((c) => {
    const first = (c.getAttribute("stroke-dasharray") ?? "").split(" ")[0];
    return first !== undefined && Number(first) > 0;
  }).length;
}

describe("GatewayDonut", () => {
  it("draws 3 visible segments for {running:5,provisioning:1,failed:2}", () => {
    const { container } = render(
      <svg>
        <GatewayDonut
          counts={{ running: 5, provisioning: 1, failed: 2 }}
          cx={50}
          cy={50}
          radius={20}
        />
      </svg>,
    );
    expect(visibleSegmentCount(container)).toBe(3);
  });

  it("total text shows '8' for {running:5,provisioning:1,failed:2}", () => {
    const { container } = render(
      <svg>
        <GatewayDonut
          counts={{ running: 5, provisioning: 1, failed: 2 }}
          cx={50}
          cy={50}
          radius={20}
        />
      </svg>,
    );
    const text = container.querySelector("text");
    expect(text?.textContent).toBe("8");
  });

  it("empty counts {} renders exactly one grey circle and shows '0'", () => {
    const { container } = render(
      <svg>
        <GatewayDonut counts={{}} cx={50} cy={50} radius={20} />
      </svg>,
    );
    const circles = container.querySelectorAll("circle");
    expect(circles.length).toBe(1);
    const text = container.querySelector("text");
    expect(text?.textContent).toBe("0");
  });

  it("zeroes a segment with 0 count: {running:3,failed:0} => 1 visible segment", () => {
    const { container } = render(
      <svg>
        <GatewayDonut
          counts={{ running: 3, failed: 0 }}
          cx={50}
          cy={50}
          radius={20}
        />
      </svg>,
    );
    expect(visibleSegmentCount(container)).toBe(1);
  });
});
