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
      documentWithMeta(
        '{"build":{"version":"v1.6.0-1234567"},"tracing":{"sampleRatio":0.25}}',
      ),
    );

    expect(config).toEqual({
      build: { version: "v1.6.0-1234567" },
      tracing: { sampleRatio: 0.25 },
    });
  });

  it("samples nothing when the meta tag is absent", () => {
    expect(readBrowserRuntimeConfig(documentWithMeta(undefined))).toEqual({
      build: {},
      tracing: { sampleRatio: 0 },
    });
  });

  it("fails closed to no tracing when the content is not valid JSON", () => {
    expect(readBrowserRuntimeConfig(documentWithMeta("not-json"))).toEqual({
      build: {},
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
        build: {},
        tracing: { sampleRatio: 0 },
      });
    }
  });

  it("ignores an invalid build version without changing tracing", () => {
    expect(
      readBrowserRuntimeConfig(
        documentWithMeta(
          '{"build":{"version":"latest"},"tracing":{"sampleRatio":0.5}}',
        ),
      ),
    ).toEqual({
      build: {},
      tracing: { sampleRatio: 0.5 },
    });
  });

  it("reads a modified local image build version", () => {
    const config = readBrowserRuntimeConfig(
      documentWithMeta(
        '{"build":{"version":"dev-abcdef0-modified"},"tracing":{"sampleRatio":0}}',
      ),
    );

    expect(config.build.version).toBe("dev-abcdef0-modified");
  });
});
