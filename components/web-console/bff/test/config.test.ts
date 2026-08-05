import { loadConfig } from "../src/config.js";

describe("loadConfig", () => {
  it("validates and normalizes trusted runtime configuration", () => {
    const config = loadConfig({
      HOST: "127.0.0.1",
      LOG_LEVEL: "warn",
      NODE_ENV: "production",
      PORT: "8081",
      STATIC_ROOT: "./public",
    });

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
});

function pathIsAbsolute(value: string): boolean {
  return value.startsWith("/");
}
