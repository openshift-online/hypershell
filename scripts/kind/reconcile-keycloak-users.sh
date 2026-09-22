#!/usr/bin/env bash
# Reconcile seed Keycloak users with the roles declared in deploy/base/keycloak.
#
# Keycloak --import-realm only creates users on first realm import. Existing Kind
# clusters keep stale realm-role mappings when keycloak.yaml adds roles (for
# example platform:admin on the admin user). The operational dashboard requires
# platform:admin (OP-DASH-04); without this step admin/admin can authenticate
# but is redirected away from /dashboard.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib.sh
source "${SCRIPT_DIR}/lib.sh"

reconcile_keycloak_seed_users
