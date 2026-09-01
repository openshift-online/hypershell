# syntax=docker/dockerfile:1
FROM registry.access.redhat.com/hi/go:1.26.7@sha256:4c7c064cc73698c13384d1445ab93595f3b9309dd89ed45b9992e67284e3ed82 AS builder

WORKDIR /workspace

COPY go.mod go.sum ./
RUN go mod download

COPY cmd/ cmd/
COPY pkg/ pkg/

RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o hsctl ./cmd/hypershell

FROM registry.access.redhat.com/hi/static:1787099997@sha256:f4d5109b57cf7eab0a7adc566f2d78f80fa0c5ec9ccab698c9fb8eb448db6071

COPY --from=builder /workspace/hsctl /usr/local/bin/

ENTRYPOINT ["/usr/local/bin/hsctl"]

LABEL org.opencontainers.image.title="HyperShell CLI" \
      org.opencontainers.image.description="Command line tool for the HyperShell API server" \
      org.opencontainers.image.version="0.0.1"
