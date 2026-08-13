FROM registry.access.redhat.com/hi/go:1.26.5-builder@sha256:fe0672d0b4bf43b502fe7ab4f251c266b558346cd3b8892c7c79b3b5f2c3a92f AS builder

WORKDIR /workspace

COPY go.mod go.sum ./
RUN go mod download

COPY cmd/ cmd/
COPY pkg/ pkg/

RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o hypershell ./cmd/hypershell

FROM registry.access.redhat.com/hi/go:1.26.5@sha256:55d95d0bd29598f821931dec7155efd331068bdd84162d7d4b2e7ad66ec6c124

COPY --from=builder --chown=1001:0 /workspace/hypershell /usr/local/bin/

USER 1001
ENTRYPOINT ["/usr/local/bin/hypershell"]

LABEL name="hypershell-cli" \
      version="0.0.1" \
      summary="HyperShell CLI" \
      description="Command line tool for the HyperShell API server"
