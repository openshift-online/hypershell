import { describe, expect, it } from "vitest";

import { readBrowserRuntimeConfig } from "./browser-runtime-config";

function documentWithMeta(content: string | undefined): Document {
  const doc = new DOMParser().parseFromString(
    "<!doctype html><html><head></head><body></body></html>",
    "text/html",
  );
  if (content !== undefined) {
    const meta = doc.createElement("meta");
    meta.setAttribute("name", "hypershell-runtime-config");
    meta.setAttribute("content", content);
    doc.head.append(meta);
  }
  return doc;
}

describe("readBrowserRuntimeConfig", () => {
  it("reads the sample ratio from the injected meta tag", () => {
    const config = readBrowserRuntimeConfig(
      documentWithMeta('{"tracing":{"sampleRatio":0.25}}'),
    );

    expect(config).toEqual({ tracing: { sampleRatio: 0.25 } });
  });

  it("samples nothing when the meta tag is absent", () => {
    expect(readBrowserRuntimeConfig(documentWithMeta(undefined))).toEqual({
      tracing: { sampleRatio: 0 },
    });
  });

  it("fails closed to no tracing when the content is not valid JSON", () => {
    expect(readBrowserRuntimeConfig(documentWithMeta("not-json"))).toEqual({
      tracing: { sampleRatio: 0 },
    });
  });

  it("fails closed when the sample ratio is out of range or missing", () => {
    for (const content of [
      '{"tracing":{"sampleRatio":5}}',
      '{"tracing":{"sampleRatio":-1}}',
      '{"tracing":{"sampleRatio":"1"}}',
      '{"tracing":{}}',
      "{}",
    ]) {
      expect(readBrowserRuntimeConfig(documentWithMeta(content))).toEqual({
        tracing: { sampleRatio: 0 },
      });
    }
  });
});
