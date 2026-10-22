#!/bin/sh
# Install only the OpenShell CLI for a remote HyperShell gateway.
set -eu

fail() {
  printf 'OpenShell CLI installation failed: %s\n' "$*" >&2
  exit 1
}

version=${OPENSHELL_VERSION:?Set OPENSHELL_VERSION to the gateway release, such as v0.0.109 or v0.0.116-rhaiv.6}
# A plain "vX.Y.Z" is a public NVIDIA/OpenShell release, downloaded and
# checksum-verified below. A suffixed version (e.g. "-rhaiv.6") names a
# downstream build that can be ahead of the last tagged public release and is
# not guaranteed proto-compatible with it -- there is no public release to
# download for it. That build is published as an image at
# quay.io/opendatahub/odh-openshell-cli under the exact same tag, so it is
# pulled and the CLI extracted from it instead.
printf '%s\n' "$version" | LC_ALL=C grep -Eq '^v[0-9]+\.[0-9]+\.[0-9]+(-[A-Za-z0-9.]+)?$' || fail "Invalid release version: $version"

install_dir=${OPENSHELL_INSTALL_DIR:-"${HOME}/.local/bin"}
mkdir -p "$install_dir"
work_dir=$(mktemp -d)
install_tmp=
trap 'rm -rf "$work_dir"; if [ -n "$install_tmp" ]; then rm -f "$install_tmp"; fi' EXIT
trap 'exit 1' HUP INT TERM

case "$version" in
  v[0-9]*.[0-9]*.[0-9]*-*)
    image="quay.io/opendatahub/odh-openshell-cli:${version}"
    host_os=$(uname -s)
    case "$host_os" in
      Linux | Darwin) ;;
      *)
        fail "This OpenShell build ($version) is only published as the Linux container image $image; supported installer hosts are Linux and macOS."
        ;;
    esac

    engine=
    for candidate in podman docker; do
      if command -v "$candidate" >/dev/null 2>&1 && "$candidate" info >/dev/null 2>&1; then
        engine=$candidate
        break
      fi
    done
    [ -n "$engine" ] || fail "Start podman or docker to install this downstream OpenShell CLI build ($image)."

    case "$host_os" in
      Linux)
        ctr_name="openshell-cli-install-$$"
        "$engine" create --name "$ctr_name" "$image" true >/dev/null 2>&1 || fail "Cannot pull $image."
        "$engine" cp "${ctr_name}:/usr/local/bin/openshell" "${work_dir}/openshell" 2>/dev/null || {
          "$engine" rm "$ctr_name" >/dev/null 2>&1
          fail "Cannot extract the openshell binary from $image."
        }
        "$engine" rm "$ctr_name" >/dev/null 2>&1
        ;;
      Darwin)
        # Downstream builds are published only as Linux container images. Keep
        # the CLI and gateway protobufs at the exact same version on macOS by
        # installing a transparent container-backed command instead of leaving
        # an older native CLI in place. The mounts preserve the state and local
        # files used by gateway login, provider ADC discovery, profile/policy
        # imports, uploads, and downloads.
        cat > "${work_dir}/openshell" <<EOF
#!/bin/sh
set -eu

image='$image'
engine=
for candidate in podman docker; do
  if command -v "\$candidate" >/dev/null 2>&1 && "\$candidate" info >/dev/null 2>&1; then
    engine=\$candidate
    break
  fi
done
if [ -z "\$engine" ]; then
  printf 'OpenShell CLI requires a running podman or docker engine to run %s.\n' "\$image" >&2
  exit 1
fi

config_root=\${XDG_CONFIG_HOME:-"\${HOME}/.config"}
openshell_config="\${config_root}/openshell"
gcloud_config=\${CLOUDSDK_CONFIG:-"\${HOME}/.config/gcloud"}
mkdir -p "\$openshell_config"

tty_flags=
if [ -t 0 ] && [ -t 1 ]; then
  tty_flags=-it
fi
userns_flag=
if [ "\$engine" = podman ]; then
  userns_flag=--userns=keep-id
fi

if [ -d "\$gcloud_config" ]; then
  exec "\$engine" run --rm \
    \${tty_flags:+"\$tty_flags"} \${userns_flag:+"\$userns_flag"} \
    -e HOME=/home/cli \
    -e CLOUDSDK_CONFIG=/home/cli/.config/gcloud \
    -e OPENSHELL_GATEWAY \
    -e OPENSHELL_GATEWAY_ENDPOINT \
    -e OPENSHELL_GATEWAY_INSECURE \
    -e OPENSHELL_WORKSPACE \
    -e OPENSHELL_COLOR \
    -v "\${openshell_config}:/home/cli/.config/openshell" \
    -v "\${gcloud_config}:/home/cli/.config/gcloud:ro" \
    -v "\${PWD}:\${PWD}" \
    -w "\${PWD}" \
    "\$image" "\$@"
fi

exec "\$engine" run --rm \
  \${tty_flags:+"\$tty_flags"} \${userns_flag:+"\$userns_flag"} \
  -e HOME=/home/cli \
  -e OPENSHELL_GATEWAY \
  -e OPENSHELL_GATEWAY_ENDPOINT \
  -e OPENSHELL_GATEWAY_INSECURE \
  -e OPENSHELL_WORKSPACE \
  -e OPENSHELL_COLOR \
  -v "\${openshell_config}:/home/cli/.config/openshell" \
  -v "\${PWD}:\${PWD}" \
  -w "\${PWD}" \
  "\$image" "\$@"
EOF
        ;;
    esac
    chmod 755 "${work_dir}/openshell"
    reported=$("${work_dir}/openshell" --version)
    [ "$reported" = "openshell ${version#v}" ] || [ "$reported" = "openshell ${version}" ] || fail "The CLI reports an unexpected version: $reported"
    ;;
  *)
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
    ;;
esac

install_tmp=$(mktemp "${install_dir}/.openshell.XXXXXX")
cat "${work_dir}/openshell" > "$install_tmp"
chmod 755 "$install_tmp"
mv -f "$install_tmp" "${install_dir}/openshell"
install_tmp=
printf 'Installed OpenShell CLI %s in %s\n' "$version" "$install_dir"
