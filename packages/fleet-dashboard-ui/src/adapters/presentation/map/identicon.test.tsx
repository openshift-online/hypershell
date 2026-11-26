import { render } from "@testing-library/react";
import { Identicon } from "./identicon";
import { ICON_BG } from "./colors";

describe("Identicon", () => {
  // Verifies: FLT-DLV-24.02
  it("draws the Initials identicon for the seed as an image", () => {
    const { container } = render(
      <svg>
        <Identicon seed="abc" x={0} y={0} size={28} />
      </svg>,
    );
    const image = container.querySelector("image");
    expect(image?.getAttribute("href")).toMatch(/^data:image\/svg\+xml/);
    expect(image?.getAttribute("data-identicon-seed")).toBe("abc");
  });

  // Verifies: FLT-DLV-24.02
  it("is deterministic - one seed, one picture; another seed differs", () => {
    const href = (seed: string) =>
      render(
        <svg>
          <Identicon seed={seed} x={0} y={0} size={28} />
        </svg>,
      )
        .container.querySelector("image")
        ?.getAttribute("href");
    expect(href("sha256:d1")).toBe(href("sha256:d1"));
    expect(href("sha256:d1")).not.toBe(href("sha256:d2"));
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
