#!/bin/sh
# Install only the OpenShell CLI for a remote HyperShell gateway.
set -eu

fail() {
  printf 'OpenShell CLI installation failed: %s\n' "$*" >&2
  exit 1
}

version=${OPENSHELL_VERSION:?Set OPENSHELL_VERSION to the gateway release, such as v0.0.109}
# The console removes distribution suffixes before it calls this script.
printf '%s\n' "$version" | LC_ALL=C grep -Eq '^v[0-9]+\.[0-9]+\.[0-9]+$' || fail "Invalid release version: $version"

case "$(uname -s)/$(uname -m)" in
  Linux/x86_64) target=x86_64-unknown-linux-musl ;;
  Linux/aarch64 | Linux/arm64) target=aarch64-unknown-linux-musl ;;
  Darwin/arm64) target=aarch64-apple-darwin ;;
  *) fail 'Supported systems: Linux x86_64, Linux arm64, and macOS arm64.' ;;
esac

for command in curl tar mktemp; do
  command -v "$command" >/dev/null 2>&1 || fail "Required command is missing: $command"
done
if command -v sha256sum >/dev/null 2>&1; then
  checksum_command=sha256sum
elif command -v shasum >/dev/null 2>&1; then
  checksum_command=shasum
else
  fail 'Install sha256sum or shasum to verify the release.'
fi

work_dir=$(mktemp -d)
install_tmp=
trap 'rm -rf "$work_dir"; if [ -n "$install_tmp" ]; then rm -f "$install_tmp"; fi' EXIT
trap 'exit 1' HUP INT TERM
asset="openshell-${target}.tar.gz"
release_url="https://github.com/NVIDIA/OpenShell/releases/download/${version}"
for file in "$asset" openshell-checksums-sha256.txt; do
  curl --proto '=https' --tlsv1.2 -fLsS --retry 3 --connect-timeout 15 --max-time 180 \
    "${release_url}/${file}" -o "${work_dir}/${file}" || fail "Cannot download $file from $version."
done
expected=$(awk -v asset="$asset" '$2 == asset { print $1 }' "${work_dir}/openshell-checksums-sha256.txt")
[ "${#expected}" -eq 64 ] || fail "The release has no unique checksum for $asset."
case "$expected" in *[!0-9a-fA-F]*) fail 'The release checksum is invalid.' ;; esac
if [ "$checksum_command" = sha256sum ]; then
  actual=$(sha256sum "${work_dir}/${asset}")
else
  actual=$(shasum -a 256 "${work_dir}/${asset}")
fi
[ "${actual%% *}" = "$expected" ] || fail "Checksum mismatch for $asset."

# Extract only the CLI. Do not install or start a local gateway service.
tar -xzf "${work_dir}/${asset}" -C "$work_dir" openshell
[ -f "${work_dir}/openshell" ] && [ ! -L "${work_dir}/openshell" ] || fail 'The release has no regular CLI file.'
chmod 755 "${work_dir}/openshell"
reported=$("${work_dir}/openshell" --version)
[ "$reported" = "openshell ${version#v}" ] || [ "$reported" = "openshell ${version}" ] || fail "The CLI reports an unexpected version: $reported"
install_dir=${OPENSHELL_INSTALL_DIR:-"${HOME}/.local/bin"}
mkdir -p "$install_dir"
install_tmp=$(mktemp "${install_dir}/.openshell.XXXXXX")
cat "${work_dir}/openshell" > "$install_tmp"
chmod 755 "$install_tmp"
mv -f "$install_tmp" "${install_dir}/openshell"
install_tmp=
printf 'Installed OpenShell CLI %s in %s\n' "$version" "$install_dir"
