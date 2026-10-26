# syntax=docker/dockerfile:1
# HI pins are the per-arch manifests from hi/go:1.26.7 and hi/static:1787099997.
# BUILDARCH selects a native Go toolchain; TARGETARCH selects the runtime and GOARCH.
# Bump amd64 and arm64 together when changing tags.
ARG BUILDARCH=amd64
ARG TARGETARCH=amd64

FROM registry.access.redhat.com/hi/go:1.26.10@sha256:e27e29ec86566046ca6e6839aecc4113d05162e3437bf66f98a00d67b7ba6fea AS go-amd64
FROM registry.access.redhat.com/hi/go:1.26.10@sha256:28bca785695e6ca43d737ae50a8ef3e0ab1b4fb949497887e204c910d6c2e9c0 AS go-arm64
FROM go-${BUILDARCH} AS builder

WORKDIR /workspace

COPY go.mod go.sum ./
RUN go mod download

COPY cmd/ cmd/
COPY pkg/ pkg/
COPY data/ data/

# All download targets travel in the same scanned image as the runnable CLI.
COPY build-release.sh ./
RUN sh build-release.sh
ARG TARGETARCH
RUN cp "dist/hsctl-linux-${TARGETARCH}" hsctl

FROM registry.access.redhat.com/hi/static:1787099997@sha256:3d43712a61ce01c7049e983e7f4c4609aaad7582c7bbd930b57372f82c446f98 AS static-amd64
FROM registry.access.redhat.com/hi/static:1787099997@sha256:2ccd532da5e8868a92f479a7268a2fce65279c1f7c8a8dd0880d399c04d6a356 AS static-arm64
FROM static-${TARGETARCH}

COPY --from=builder /workspace/hsctl /usr/local/bin/
COPY --from=builder /workspace/dist/ /releases/

ENTRYPOINT ["/usr/local/bin/hsctl"]

LABEL org.opencontainers.image.title="HyperShell CLI" \
      org.opencontainers.image.description="Command line tool for the HyperShell API server" \
      org.opencontainers.image.version="0.0.1"
