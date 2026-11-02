import { describe, expect, it } from "vitest";

import type { SpecNode } from "../domain/spec-document";
import { ID_LABEL_ATTRIBUTE, labelIds } from "./id-labels-html";

const node = (id: string, kind: SpecNode["kind"]): SpecNode =>
  ({ id: `spec.md#${id}`, kind, specId: id }) as SpecNode;

const known = new Map([
  ["QRX-01", node("QRX-01", "spec")],
  ["QRX-01.02", node("QRX-01.02", "scenario")],
]);
const resolve = (id: string) => known.get(id);

const labels = (html: string) => {
  const template = document.createElement("template");
  template.innerHTML = labelIds(html, resolve);
  return [...template.content.querySelectorAll(`[${ID_LABEL_ATTRIBUTE}]`)].map(
    (button) => [button.textContent, button.getAttribute(ID_LABEL_ATTRIBUTE)],
  );
};

describe("labelIds", () => {
  it("labels known IDs, the longest match first", () => {
    expect(labels("<p>See QRX-01 and QRX-01.02.</p>")).toEqual([
      ["QRX-01", "spec.md#QRX-01"],
      ["QRX-01.02", "spec.md#QRX-01.02"],
    ]);
  });

  it("leaves unknown IDs, words with IDs inside, links and code blocks", () => {
    expect(
      labels(
        "<p>QRX-02 XQRX-01 QRX-01X <a>QRX-01</a></p><pre><code>QRX-01</code></pre>",
      ),
    ).toEqual([]);
  });

  it("replaces inline code that holds only an ID", () => {
    const html = labelIds(
      "<p><code>QRX-01</code> and <code>QRX-01 x</code></p>",
      resolve,
    );
    expect(html).not.toContain("<code>QRX-01</code>");
    expect(html).toContain("<code><span");
  });

  it("marks requirement labels darker than scenario labels", () => {
    const html = labelIds("<p>QRX-01 QRX-01.02</p>", resolve);
    expect(html.match(/sv-id-label--requirement/gu)).toHaveLength(1);
  });
});
