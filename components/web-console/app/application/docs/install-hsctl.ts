import {
  externalLinkUrls,
  shellArgument,
} from "@openshift-online/hypershell-gateway-management-ui";

/** Operating system tabs of the Install hsctl page (WEB-UDOC-08). */
export type InstallOs = "linux" | "macos";

export const installOsValues: readonly InstallOs[] = ["macos", "linux"];

/** Validates the `os` search parameter; an invalid value is ignored. */
export function parseInstallOs(
  value: string | null | undefined,
): InstallOs | undefined {
  return value === "macos" || value === "linux" ? value : undefined;
}

export interface BrowserPlatformHints {
  readonly platform?: string;
  readonly userAgent?: string;
  readonly userAgentDataPlatform?: string;
}

function osFromHint(hint: string | undefined): InstallOs | undefined {
  if (hint === undefined || hint.trim() === "") {
    return undefined;
  }
  const value = hint.toLowerCase();
  if (/mac|darwin/u.test(value) && !/iphone|ipad|ipod/u.test(value)) {
    return "macos";
  }
  if (/linux|x11|ubuntu|fedora/u.test(value) && !value.includes("android")) {
    return "linux";
  }
  return undefined;
}

/**
 * Detects the reader's operating system from browser hints, in the order the
 * spec names: `navigator.userAgentData.platform`, then `navigator.platform`,
 * then `navigator.userAgent`. The first hint that names macOS or Linux wins;
 * when none does, the page selects Linux.
 */
export function detectInstallOs(hints: BrowserPlatformHints): InstallOs {
  for (const hint of [
    hints.userAgentDataPlatform,
    hints.platform,
    hints.userAgent,
  ]) {
    const os = osFromHint(hint);
    if (os !== undefined) {
      return os;
    }
  }
  return "linux";
}

/** Release assets for each tab (CLI-REL-01). */
export const hsctlReleaseAssets = {
  linux: { amd64: "hsctl-linux-amd64", arm64: "hsctl-linux-arm64" },
  macos: { amd64: "hsctl-darwin-amd64", arm64: "hsctl-darwin-arm64" },
} as const;

export const hsctlWindowsAsset = "hsctl-windows-amd64.exe";

/** Download base used when `HSCTL_DOWNLOAD_BASE_URL` is not configured. */
export const hsctlReleaseDownloadPrefix = `${externalLinkUrls.hyperShellReleases}/download`;

/**
 * Builds the install command block (WEB-UDOC-08): find the CPU architecture
 * with `uname -m`, download the matching asset and `SHA256SUMS` from one
 * release, verify the checksum before anything is installed, install as
 * `hsctl` in `~/.local/bin`, add that directory to `PATH` for this shell, and
 * run `hsctl version`. It never uses GitHub's `latest` release, because
 * publication does not move it (CLI-REL-03). The block avoids `set -e` and
 * `exit`, so a failure never closes the reader's shell.
 */
export function buildInstallHsctlCommand(
  os: InstallOs,
  downloadBaseUrl: string | undefined,
): string {
  const assetOs = os === "macos" ? "darwin" : "linux";
  const verify = os === "macos" ? "shasum -a 256 -c -" : "sha256sum -c -";
  // A configured location is one shell word, so no character in it can
  // expand; without one, the release tag comes from the reader's shell.
  const urlAssignment =
    downloadBaseUrl === undefined
      ? `HSCTL_URL="${hsctlReleaseDownloadPrefix}/\${HSCTL_RELEASE:?Set HSCTL_RELEASE to your release tag}"`
      : `HSCTL_URL=${shellArgument(downloadBaseUrl)}`;
  const lines = [
    ...(downloadBaseUrl === undefined
      ? [
          "# First set HSCTL_RELEASE to the release tag that matches your HyperShell deployment:",
          "# export HSCTL_RELEASE=<release tag>",
        ]
      : []),
    'case "$(uname -m)" in',
    "  x86_64|amd64) HSCTL_ARCH=amd64 ;;",
    "  arm64|aarch64) HSCTL_ARCH=arm64 ;;",
    '  *) HSCTL_ARCH=""; echo "hsctl: unsupported CPU architecture: $(uname -m)" >&2 ;;',
    "esac",
    'if [ -n "$HSCTL_ARCH" ]; then',
    `  HSCTL_ASSET="hsctl-${assetOs}-$HSCTL_ARCH"`,
    `  ${urlAssignment}`,
    '  HSCTL_TMP="$(mktemp -d)"',
    '  if curl -fsSLo "$HSCTL_TMP/$HSCTL_ASSET" "$HSCTL_URL/$HSCTL_ASSET" \\',
    '    && curl -fsSLo "$HSCTL_TMP/SHA256SUMS" "$HSCTL_URL/SHA256SUMS" \\',
    `    && (cd "$HSCTL_TMP" && grep "  $HSCTL_ASSET\\$" SHA256SUMS | ${verify}); then`,
    '    chmod +x "$HSCTL_TMP/$HSCTL_ASSET"',
    '    mkdir -p "$HOME/.local/bin"',
    '    mv "$HSCTL_TMP/$HSCTL_ASSET" "$HOME/.local/bin/hsctl"',
    '    export PATH="$HOME/.local/bin:$PATH"',
    "    hsctl version",
    "  else",
    '    echo "hsctl: download or checksum verification failed; nothing was installed" >&2',
    "  fi",
    '  rm -rf "$HSCTL_TMP"',
    "fi",
  ];
  return lines.join("\n");
}
