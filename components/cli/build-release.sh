#!/bin/sh
set -eu
mkdir -p dist
for target in ${HSCTL_RELEASE_TARGETS:-linux-amd64 linux-arm64 darwin-amd64 darwin-arm64 windows-amd64}; do
    os=${target%-*}
    arch=${target#*-}
    suffix=
    if [ "$os" = windows ]; then suffix=.exe; fi
    CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" go build -trimpath -ldflags="-s -w" \
        -o "dist/hsctl-${target}${suffix}" ./cmd/hsctl
done
