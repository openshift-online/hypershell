FROM registry.access.redhat.com/hi/go:1.26.5-builder@sha256:aef155087940a9221c59ca48ad8af371a4c1a46b05f0283737149cb00ef484b3 AS builder

WORKDIR /workspace

COPY go.mod go.sum ./
RUN go mod download

COPY cmd/ cmd/
COPY pkg/ pkg/

RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o hypershell ./cmd/hypershell

FROM registry.access.redhat.com/hi/go:1.26.5@sha256:eb29e565caf76cf408a77a73548a3ed62e00e3516c17d02d5f36838aea69a605

COPY --from=builder --chown=1001:0 /workspace/hypershell /usr/local/bin/

USER 1001
ENTRYPOINT ["/usr/local/bin/hypershell"]

LABEL name="hypershell-cli" \
      version="0.0.1" \
      summary="HyperShell CLI" \
      description="Command line tool for the HyperShell API server"
