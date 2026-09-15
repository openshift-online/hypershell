#!/usr/bin/env bash
# Run the controller database tests against a disposable TLS PostgreSQL server.
set -euo pipefail
ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
ENGINE=${CONTAINER_ENGINE:-$(command -v podman || command -v docker)}
FIXTURE=$(mktemp -d)
NAME="hypershell-database-test-$$"
cleanup() {
  "$ENGINE" rm -f "$NAME" >/dev/null 2>&1 || true
  rm -rf "$FIXTURE"
}
trap cleanup EXIT
bash "$ROOT/tests/fixtures/controller-database/generate.sh" "$FIXTURE"
"$ENGINE" run -d --name "$NAME" -p 127.0.0.1::5432 \
  -e POSTGRES_PASSWORD=fixture-root --entrypoint sh \
  -v "$FIXTURE:/fixture:Z" \
  -v "$FIXTURE/init.sql:/docker-entrypoint-initdb.d/init.sql:Z" \
  docker.io/library/postgres:18@sha256:a02db8cac496f15b094798a38254f14d6e00741f709360e5e00bb6668ea31636 \
  -c 'cp /fixture/server.key /tmp/server.key && chown postgres:postgres /tmp/server.key && chmod 600 /tmp/server.key && exec docker-entrypoint.sh postgres -c ssl=on -c ssl_cert_file=/fixture/server.crt -c ssl_key_file=/tmp/server.key' \
  >/dev/null
for _ in $(seq 1 60); do
  if "$ENGINE" exec "$NAME" pg_isready -h 127.0.0.1 -U postgres >/dev/null 2>&1; then break; fi
  sleep 1
done
"$ENGINE" exec "$NAME" pg_isready -h 127.0.0.1 -U postgres >/dev/null
export TEST_DATABASE_PORT
TEST_DATABASE_PORT=$("$ENGINE" port "$NAME" 5432/tcp | awk -F: '{print $NF}')
export TEST_DATABASE_CA="$FIXTURE/ca.crt"
cd "$ROOT/components/control-plane"
go test -tags=integration -count=1 -v ./internal/gateway -run '^TestControllerDatabaseLifecycle$'
