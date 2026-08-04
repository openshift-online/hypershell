FROM registry.access.redhat.com/ubi9/go-toolset:1.26@sha256:46376c6723c3a4961a165c2768461e7fac48a79932cdde0a6e6a57724ef61ba0 AS builder

USER root

WORKDIR /workspace

COPY rh-trex-ai/ /rh-trex-ai/

COPY hypershell/components/api-server/go.mod hypershell/components/api-server/go.sum ./
RUN go mod download

COPY hypershell/components/api-server/cmd/ cmd/
COPY hypershell/components/api-server/pkg/ pkg/
COPY hypershell/components/api-server/plugins/ plugins/
COPY hypershell/components/api-server/openapi/ openapi/

ARG GIT_VERSION=unknown
ARG BUILD_TIME=unknown
RUN go build -ldflags="-s -w \
  -X github.com/openshift-online/hypershell/components/api-server/pkg/api.Version=${GIT_VERSION} \
  -X 'github.com/openshift-online/hypershell/components/api-server/pkg/api.BuildTime=${BUILD_TIME}'" \
  -o hypershell ./cmd/hypershell

FROM registry.access.redhat.com/ubi9/ubi-minimal:9.8@sha256:48fa5d8cda7fc00d270d8747c3eaa54ae196f0820d8540074a9c8c61d5e3056f

WORKDIR /

COPY --from=builder /workspace/hypershell /usr/local/bin/

EXPOSE 8000
EXPOSE 9000

ENTRYPOINT ["/usr/local/bin/hypershell"]
CMD ["serve"]

LABEL name="hypershell" \
      version="0.0.1" \
      summary="HyperShell API Server" \
      description="REST + gRPC API server for HyperShell fleet management"
