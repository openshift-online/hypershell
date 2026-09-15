#!/usr/bin/env bash
# Generate disposable test credentials. These files must not enter Git.
set -euo pipefail
DIR=${1:?fixture directory required}
mkdir -p "$DIR"
chmod 755 "$DIR"
openssl req -x509 -newkey rsa:2048 -nodes -days 2 -subj /CN=fixture-ca \
  -keyout "$DIR/ca.key" -out "$DIR/ca.crt" >/dev/null 2>&1
openssl req -newkey rsa:2048 -nodes -subj /CN=localhost \
  -keyout "$DIR/server.key" -out "$DIR/server.csr" >/dev/null 2>&1
printf '%s\n' 'subjectAltName=DNS:localhost,DNS:gateway-database.database-secret-test.svc' > "$DIR/extensions"
openssl x509 -req -in "$DIR/server.csr" -CA "$DIR/ca.crt" -CAkey "$DIR/ca.key" \
  -CAcreateserial -days 2 -extfile "$DIR/extensions" -out "$DIR/server.crt" >/dev/null 2>&1
# The test container copies the server key to a private file owned by PostgreSQL.
chmod 644 "$DIR/server.crt" "$DIR/ca.crt"
chmod 600 "$DIR/server.key"
cat > "$DIR/init.sql" <<'SQL'
CREATE ROLE provisioner LOGIN CREATEDB CREATEROLE PASSWORD 'fixture-admin';
GRANT pg_signal_backend TO provisioner;
GRANT CREATE ON SCHEMA public TO provisioner;
SQL
