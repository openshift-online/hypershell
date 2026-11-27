import { spawnSync } from "node:child_process";
import { createHash } from "node:crypto";
import {
  chmodSync,
  existsSync,
  mkdirSync,
  mkdtempSync,
  rmSync,
  writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";

import { afterEach, describe, expect, it } from "vitest";

import {
  buildInstallHsctlCommand,
  detectInstallOs,
  hsctlReleaseDownloadPrefix,
  parseInstallOs,
} from "./install-hsctl";

const releaseUrl =
  "https://github.com/openshift-online/hypershell/releases/download/release-bundle-1";

describe("install hsctl", () => {
  // Verifies: WEB-UDOC-08, WEB-UDOC-08.01
  it("detects macOS and Linux in the documented order", () => {
    expect(detectInstallOs({ userAgentDataPlatform: "macOS" })).toBe("macos");
    expect(
      detectInstallOs({ platform: "MacIntel", userAgent: "X11; Linux" }),
    ).toBe("macos");
    expect(detectInstallOs({ platform: "Linux x86_64" })).toBe("linux");
    expect(
      detectInstallOs({
        platform: "",
        userAgent: "Mozilla/5.0 (X11; Fedora; Linux x86_64)",
      }),
    ).toBe("linux");
    expect(
      detectInstallOs({
        userAgentDataPlatform: "Windows",
        platform: "Win32",
        userAgent: "Mozilla/5.0 (Windows NT 10.0)",
      }),
    ).toBe("linux");
    expect(
      detectInstallOs({ userAgent: "Mozilla/5.0 (iPhone; CPU iPhone OS)" }),
    ).toBe("linux");
  });

  // Verifies: WEB-UDOC-08
  it("validates the os search parameter", () => {
    expect(parseInstallOs("macos")).toBe("macos");
    expect(parseInstallOs("linux")).toBe("linux");
    expect(parseInstallOs("windows")).toBeUndefined();
    expect(parseInstallOs(null)).toBeUndefined();
  });

  // Verifies: WEB-UDOC-08.02
  it("downloads from the configured release and verifies before installing", () => {
    const command = buildInstallHsctlCommand("linux", releaseUrl);
    expect(command).toContain(`HSCTL_URL=${releaseUrl}`);
    expect(command).toContain('HSCTL_ASSET="hsctl-linux-$HSCTL_ARCH"');
    expect(command).toContain('"$HSCTL_URL/SHA256SUMS"');
    expect(command.indexOf("sha256sum -c -")).toBeLessThan(
      command.indexOf("chmod +x"),
    );
    expect(command).toContain('export PATH="$HOME/.local/bin:$PATH"');
    expect(command).toContain("hsctl version");
    expect(command).not.toContain("latest");
    expect(command).not.toMatch(/set -e|\bexit\b/u);
    expect(buildInstallHsctlCommand("macos", releaseUrl)).toContain(
      "shasum -a 256 -c -",
    );
    expect(buildInstallHsctlCommand("macos", releaseUrl)).toContain(
      'HSCTL_ASSET="hsctl-darwin-$HSCTL_ARCH"',
    );
  });

  // Verifies: WEB-UDOC-08.03
  it("reads the release tag from HSCTL_RELEASE when no location is configured", () => {
    const command = buildInstallHsctlCommand("linux", undefined);
    expect(command).toContain(
      `HSCTL_URL="${hsctlReleaseDownloadPrefix}/\${HSCTL_RELEASE:?`,
    );
    expect(command).not.toContain("latest");
  });

  // Verifies: WEB-UDOC-10
  it("keeps a configured location one shell word", () => {
    expect(
      buildInstallHsctlCommand(
        "linux",
        "https://example.com/releases/$(touch /tmp/x)",
      ),
    ).toContain("HSCTL_URL='https://example.com/releases/$(touch /tmp/x)'");
  });
});

const hasShellTools =
  spawnSync("bash", ["-c", "command -v sha256sum"]).status === 0;

describe.runIf(hasShellTools)("install hsctl command in a shell", () => {
  let root: string | undefined;

  afterEach(() => {
    if (root !== undefined) {
      rmSync(root, { force: true, recursive: true });
      root = undefined;
    }
  });

  function runInstall(options: { corruptChecksum?: boolean; arch?: string }) {
    root = mkdtempSync(path.join(tmpdir(), "hsctl-install-"));
    const home = path.join(root, "home");
    const release = path.join(root, "release");
    const bin = path.join(root, "bin");
    mkdirSync(home);
    mkdirSync(release);
    mkdirSync(bin);
    const asset = "#!/bin/sh\necho 'hsctl version test-1'\n";
    writeFileSync(path.join(release, "hsctl-linux-amd64"), asset);
    const digest = createHash("sha256")
      .update(options.corruptChecksum ? "tampered" : asset)
      .digest("hex");
    writeFileSync(
      path.join(release, "SHA256SUMS"),
      `${digest}  hsctl-linux-amd64\n${"0".repeat(64)}  hsctl-linux-arm64\n`,
    );
    // Stand-ins for curl and uname: curl copies from the local release
    // directory, and uname reports the requested CPU architecture.
    writeFileSync(
      path.join(bin, "curl"),
      `#!/bin/sh\nout=""\nurl=""\nwhile [ $# -gt 0 ]; do\n  case "$1" in\n    -fsSLo) out="$2"; shift 2 ;;\n    *) url="$1"; shift ;;\n  esac\ndone\ncp "${release}/$(basename "$url")" "$out"\n`,
    );
    writeFileSync(
      path.join(bin, "uname"),
      `#!/bin/sh\necho ${options.arch ?? "x86_64"}\n`,
    );
    chmodSync(path.join(bin, "curl"), 0o755);
    chmodSync(path.join(bin, "uname"), 0o755);
    const result = spawnSync(
      "bash",
      ["-c", buildInstallHsctlCommand("linux", "https://example.test/rel")],
      {
        encoding: "utf8",
        env: { HOME: home, PATH: `${bin}:/usr/bin:/bin` },
      },
    );
    return { home, result };
  }

  // Verifies: WEB-UDOC-08.02
  it("installs a verified asset and runs hsctl version", () => {
    const { home, result } = runInstall({});
    expect(result.stderr).toBe("");
    expect(result.stdout).toContain("hsctl version test-1");
    expect(existsSync(path.join(home, ".local/bin/hsctl"))).toBe(true);
  });

  // Verifies: WEB-UDOC-08.02
  it("installs nothing when the checksum does not match", () => {
    const { home, result } = runInstall({ corruptChecksum: true });
    expect(result.stderr).toContain("nothing was installed");
    expect(existsSync(path.join(home, ".local/bin/hsctl"))).toBe(false);
  });

  // Verifies: WEB-UDOC-08
  it("stops with a clear message for an unsupported architecture", () => {
    const { home, result } = runInstall({ arch: "riscv64" });
    expect(result.stderr).toContain("unsupported CPU architecture: riscv64");
    expect(existsSync(path.join(home, ".local/bin/hsctl"))).toBe(false);
  });
});
