import { render } from "@testing-library/react";
import { Identicon } from "./identicon";
import { ICON_BG } from "./colors";

describe("Identicon", () => {
  it("renders at least 2 rect elements", () => {
    const { container } = render(
      <svg>
        <Identicon seed="abc" x={0} y={0} size={28} />
      </svg>,
    );
    const rects = container.querySelectorAll("rect");
    expect(rects.length).toBeGreaterThanOrEqual(2);
  });

  it("is deterministic - same seed yields same number of rects", () => {
    const { container: c1 } = render(
      <svg>
        <Identicon seed="deterministic-test" x={0} y={0} size={28} />
      </svg>,
    );
    const { container: c2 } = render(
      <svg>
        <Identicon seed="deterministic-test" x={0} y={0} size={28} />
      </svg>,
    );
    expect(c1.querySelectorAll("rect").length).toBe(
      c2.querySelectorAll("rect").length,
    );
  });

  it("first rect is the white background matching ICON_BG", () => {
    const { container } = render(
      <svg>
        <Identicon seed="bg-test" x={0} y={0} size={28} />
      </svg>,
    );
    const rects = container.querySelectorAll("rect");
    const first = rects[0];
    expect(first).toBeDefined();
    expect(first?.getAttribute("fill")).toBe(ICON_BG);
  });
});
