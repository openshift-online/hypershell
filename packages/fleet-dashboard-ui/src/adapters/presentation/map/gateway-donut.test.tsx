import { render } from "@testing-library/react";
import { GatewayDonut } from "./gateway-donut";

describe("GatewayDonut", () => {
  it("renders 3 segment circles for {running:5,provisioning:1,failed:2}", () => {
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
    // The rotate group contains only the segment circles
    const rotateGroup = container.querySelector("g[transform]");
    const circles =
      rotateGroup?.querySelectorAll("circle") ??
      container.querySelectorAll("circle");
    expect(circles.length).toBe(3);
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

  it("skips a segment with 0 count: {running:3,failed:0} => 1 segment circle", () => {
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
    const rotateGroup = container.querySelector("g[transform]");
    const circles =
      rotateGroup?.querySelectorAll("circle") ??
      container.querySelectorAll("circle");
    expect(circles.length).toBe(1);
  });
});
