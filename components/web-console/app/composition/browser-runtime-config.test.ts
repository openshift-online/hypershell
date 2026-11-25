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
  it("reads the sample ratio and build metadata from the meta tag", () => {
    const config = readBrowserRuntimeConfig(
      documentWithMeta(
        '{"apiVersion":"abc1234","tracing":{"sampleRatio":0.25},"webVersion":"def5678","webBuildTime":"2026-11-19 20:00:00 UTC"}',
      ),
    );

    expect(config).toEqual({
      apiVersion: "abc1234",
      tracing: { sampleRatio: 0.25 },
      webVersion: "def5678",
      webBuildTime: "2026-11-19 20:00:00 UTC",
    });
  });

  it("defaults build metadata to unknown without disabling tracing", () => {
    for (const content of [
      '{"tracing":{"sampleRatio":0.25}}',
      '{"apiVersion":5,"tracing":{"sampleRatio":0.25},"webVersion":"","webBuildTime":""}',
    ]) {
      expect(readBrowserRuntimeConfig(documentWithMeta(content))).toEqual({
        apiVersion: "unknown",
        tracing: { sampleRatio: 0.25 },
        webVersion: "unknown",
        webBuildTime: "unknown",
      });
    }
  });

  it("samples nothing when the meta tag is absent", () => {
    expect(readBrowserRuntimeConfig(documentWithMeta(undefined))).toEqual({
      apiVersion: "unknown",
      tracing: { sampleRatio: 0 },
      webVersion: "unknown",
      webBuildTime: "unknown",
    });
  });

  it("fails closed to no tracing when the content is not valid JSON", () => {
    expect(readBrowserRuntimeConfig(documentWithMeta("not-json"))).toEqual({
      apiVersion: "unknown",
      tracing: { sampleRatio: 0 },
      webVersion: "unknown",
      webBuildTime: "unknown",
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
        apiVersion: "unknown",
        tracing: { sampleRatio: 0 },
        webVersion: "unknown",
        webBuildTime: "unknown",
      });
    }
  });
  // Verifies: WEB-UDOC-08, WEB-UDOC-08.02
  it("reads a valid hsctl release download location", () => {
    const config = readBrowserRuntimeConfig(
      documentWithMeta(
        JSON.stringify({
          apiVersion: "abc1234",
          hsctl_download_base_url:
            "https://github.com/openshift-online/hypershell/releases/download/release-1/",
          tracing: { sampleRatio: 0 },
          webBuildTime: "unknown",
          webVersion: "unknown",
        }),
      ),
    );

    expect(config.hsctlDownloadBaseUrl).toBe(
      "https://github.com/openshift-online/hypershell/releases/download/release-1",
    );
  });

  // Verifies: WEB-UDOC-08.03
  it.each([
    ["absent", undefined],
    ["not HTTPS", "http://example.com/releases/download/v1"],
    ["carrying a query", "https://example.com/releases?tag=latest"],
    ["carrying a fragment", "https://example.com/releases#v1"],
    ["not a URL", "release-1"],
    ["not a string", 42],
  ])("ignores a download location that is %s", (_label, value) => {
    const config = readBrowserRuntimeConfig(
      documentWithMeta(
        JSON.stringify({
          hsctl_download_base_url: value,
          tracing: { sampleRatio: 0 },
        }),
      ),
    );

    expect(config.hsctlDownloadBaseUrl).toBeUndefined();
  });
});
