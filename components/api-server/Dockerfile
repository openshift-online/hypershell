FROM registry.access.redhat.com/hi/go:1.26.5-builder@sha256:fe0672d0b4bf43b502fe7ab4f251c266b558346cd3b8892c7c79b3b5f2c3a92f AS builder

WORKDIR /workspace

COPY go.mod go.sum ./
RUN go mod edit -dropreplace github.com/openshift-online/rh-trex-ai && go mod download

COPY cmd/ cmd/
COPY pkg/ pkg/
COPY plugins/ plugins/
COPY openapi/ openapi/

ARG GIT_VERSION=unknown
ARG BUILD_TIME=unknown
RUN CGO_ENABLED=0 go build -mod=mod -ldflags="-s -w \
  -X github.com/openshift-online/hypershell/components/api-server/pkg/api.Version=${GIT_VERSION} \
  -X 'github.com/openshift-online/hypershell/components/api-server/pkg/api.BuildTime=${BUILD_TIME}'" \
  -o hypershell ./cmd/hypershell

FROM registry.access.redhat.com/hi/go:1.26.5@sha256:55d95d0bd29598f821931dec7155efd331068bdd84162d7d4b2e7ad66ec6c124

COPY --from=builder --chown=1001:0 /workspace/hypershell /usr/local/bin/

USER 1001
EXPOSE 8000
EXPOSE 9000

ENTRYPOINT ["/usr/local/bin/hypershell"]
CMD ["serve"]

LABEL name="hypershell" \
      version="0.0.1" \
      summary="HyperShell API Server" \
      description="REST + gRPC API server for HyperShell fleet management"
