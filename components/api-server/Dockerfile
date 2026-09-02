# syntax=docker/dockerfile:1
ARG BUILD_PREFIX=dev
ARG VCS_REF=unknown

FROM registry.access.redhat.com/hi/go:1.26.7@sha256:4c7c064cc73698c13384d1445ab93595f3b9309dd89ed45b9992e67284e3ed82 AS builder

WORKDIR /workspace

COPY components/api-server/go.mod components/api-server/go.sum ./
RUN go mod edit -dropreplace github.com/openshift-online/rh-trex-ai && go mod download

COPY components/api-server/cmd/ cmd/
COPY components/api-server/pkg/ pkg/
COPY components/api-server/plugins/ plugins/
COPY components/api-server/openapi/ openapi/

ARG BUILD_PREFIX
ARG VCS_REF
ARG BUILD_TIME=unknown
RUN set -eu; \
  short_ref="${VCS_REF%?????????????????????????????????}"; \
  build_version="${BUILD_PREFIX}-${short_ref}"; \
  build_time="${BUILD_TIME}"; \
  if [ "${build_time}" = "unknown" ]; then build_time="$(date -u '+%Y-%m-%dT%H:%M:%SZ')"; fi; \
  CGO_ENABLED=0 go build -mod=mod -ldflags="-s -w \
    -X github.com/openshift-online/rh-trex-ai/pkg/api.Version=${build_version} \
    -X 'github.com/openshift-online/rh-trex-ai/pkg/api.BuildTime=${build_time}'" \
    -o hypershell ./cmd/hypershell

FROM registry.access.redhat.com/hi/static:1787099997@sha256:f4d5109b57cf7eab0a7adc566f2d78f80fa0c5ec9ccab698c9fb8eb448db6071

ARG BUILD_PREFIX
ARG VCS_REF

COPY --from=builder /workspace/hypershell /usr/local/bin/

ENV HYPERSHELL_BUILD_VERSION="${BUILD_PREFIX}-${VCS_REF%?????????????????????????????????}" \
    HYPERSHELL_BUILD_REVISION="${VCS_REF}"

EXPOSE 8000
EXPOSE 9000

ENTRYPOINT ["/usr/local/bin/hypershell"]
CMD ["serve"]

LABEL org.opencontainers.image.title="HyperShell API Server" \
      org.opencontainers.image.description="REST + gRPC API server for HyperShell fleet management" \
      org.opencontainers.image.version="${BUILD_PREFIX}-${VCS_REF%?????????????????????????????????}" \
      org.opencontainers.image.revision="${VCS_REF}"
