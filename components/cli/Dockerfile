# syntax=docker/dockerfile:1
FROM registry.access.redhat.com/hi/go:1.26.5@sha256:eb29e565caf76cf408a77a73548a3ed62e00e3516c17d02d5f36838aea69a605 AS builder

WORKDIR /workspace

COPY go.mod go.sum ./
RUN go mod download

COPY cmd/ cmd/
COPY pkg/ pkg/

RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o hypershell ./cmd/hypershell

FROM registry.access.redhat.com/hi/static:latest@sha256:f4d5109b57cf7eab0a7adc566f2d78f80fa0c5ec9ccab698c9fb8eb448db6071

COPY --from=builder /workspace/hypershell /usr/local/bin/

ENTRYPOINT ["/usr/local/bin/hypershell"]

LABEL name="hypershell-cli" \
      version="0.0.1" \
      summary="HyperShell CLI" \
      description="Command line tool for the HyperShell API server"
