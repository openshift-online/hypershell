import { browserRuntimeConfig, loadConfig } from "../src/config.js";

describe("loadConfig", () => {
  it("validates and normalizes trusted runtime configuration", () => {
    const config = loadConfig({
      HOST: "127.0.0.1",
      HYPERSHELL_API_ORIGIN: "https://api.example.test/",
      HYPERSHELL_API_TIMEOUT_MS: "5000",
      LOG_LEVEL: "warn",
      NODE_ENV: "production",
      PORT: "8081",
      STATIC_ROOT: "./public",
    });

    expect(config.apiOrigin).toBe("https://api.example.test");
    expect(config.apiTimeoutMs).toBe(5000);
    expect(config.host).toBe("127.0.0.1");
    expect(config.port).toBe(8081);
    expect(pathIsAbsolute(config.staticRoot)).toBe(true);
  });

  it("fails closed without echoing invalid values", () => {
    expect(() => loadConfig({ PORT: "secret-invalid-port" })).toThrow(
      /Invalid web-console BFF configuration/,
    );
    expect(() => loadConfig({ PORT: "secret-invalid-port" })).not.toThrow(
      /secret-invalid-port/,
    );
  });

  it("rejects an API URL that is not an origin", () => {
    expect(() =>
      loadConfig({ HYPERSHELL_API_ORIGIN: "https://api.example.test/v1" }),
    ).toThrow(/HYPERSHELL_API_ORIGIN/u);
  });

  it("rejects a Prometheus URL that is not an origin", () => {
    expect(() =>
      loadConfig({ PROMETHEUS_URL: "http://127.0.0.1:9090/metrics" }),
    ).toThrow(/PROMETHEUS_URL/u);
  });

  it("normalizes the Prometheus origin", () => {
    const config = loadConfig({
      PROMETHEUS_URL: "http://127.0.0.1:9090/",
      STATIC_ROOT: "./public",
    });

    expect(config.prometheusUrl).toBe("http://127.0.0.1:9090");
  });

  it("accepts a custom Prometheus query timeout", () => {
    const config = loadConfig({
      PROMETHEUS_QUERY_TIMEOUT_MS: "15000",
      STATIC_ROOT: "./public",
    });

    expect(config.prometheusQueryTimeoutMs).toBe(15_000);
  });

  it("defaults the Prometheus query timeout to ten seconds", () => {
    const config = loadConfig({ STATIC_ROOT: "./public" });

    expect(config.prometheusQueryTimeoutMs).toBe(10_000);
  });

  it("leaves tracing disabled when no collector endpoint is set", () => {
    const config = loadConfig({ STATIC_ROOT: "./public" });

    expect(config.tracing).toBeUndefined();
  });

  it("derives the OTLP traces endpoint and sampling from configuration", () => {
    const config = loadConfig({
      OTEL_EXPORTER_OTLP_ENDPOINT: "http://collector.example.test:4318/",
      OTEL_SERVICE_NAME: "web-console-bff",
      OTEL_TRACES_SAMPLE_RATIO: "0.25",
      STATIC_ROOT: "./public",
    });

    expect(config.tracing).toEqual({
      collectorEndpoint: "http://collector.example.test:4318/",
      sampleRatio: 0.25,
      serviceName: "web-console-bff",
      tracesEndpoint: "http://collector.example.test:4318/v1/traces",
    });
  });

  it("defaults the service name and sample ratio when only the endpoint is set", () => {
    const config = loadConfig({
      OTEL_EXPORTER_OTLP_ENDPOINT: "http://collector.example.test:4318",
      STATIC_ROOT: "./public",
    });

    expect(config.tracing?.serviceName).toBe("hypershell-web-console-bff");
    expect(config.tracing?.sampleRatio).toBe(1);
    expect(config.tracing?.tracesEndpoint).toBe(
      "http://collector.example.test:4318/v1/traces",
    );
  });

  it("rejects an out-of-range trace sample ratio", () => {
    expect(() =>
      loadConfig({
        OTEL_EXPORTER_OTLP_ENDPOINT: "http://collector.example.test:4318",
        OTEL_TRACES_SAMPLE_RATIO: "5",
        STATIC_ROOT: "./public",
      }),
    ).toThrow(/OTEL_TRACES_SAMPLE_RATIO/u);
  });
});

describe("browserRuntimeConfig", () => {
  it("mirrors the configured sample ratio to the browser allowlist", () => {
    const config = loadConfig({
      OTEL_EXPORTER_OTLP_ENDPOINT: "http://collector.example.test:4318",
      OTEL_TRACES_SAMPLE_RATIO: "0.25",
      STATIC_ROOT: "./public",
    });

    expect(browserRuntimeConfig(config)).toEqual({
      tracing: { sampleRatio: 0.25 },
    });
  });

  it("samples nothing in the browser when tracing is disabled", () => {
    const config = loadConfig({ STATIC_ROOT: "./public" });

    expect(browserRuntimeConfig(config)).toEqual({
      tracing: { sampleRatio: 0 },
    });
  });

  it("exposes no server-only configuration to the browser", () => {
    const config = loadConfig({
      OTEL_EXPORTER_OTLP_ENDPOINT: "http://collector.example.test:4318",
      SESSION_SECRET: "a".repeat(64),
      STATIC_ROOT: "./public",
    });

    const serialized = JSON.stringify(browserRuntimeConfig(config));
    expect(serialized).not.toContain("collector.example.test");
    expect(serialized).not.toContain("a".repeat(64));
    expect(Object.keys(browserRuntimeConfig(config))).toEqual(["tracing"]);
  });
});

function pathIsAbsolute(value: string): boolean {
  return value.startsWith("/");
}
