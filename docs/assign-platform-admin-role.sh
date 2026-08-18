#!/usr/bin/env bash
# assign-platform-admin-role.sh - Grant platform:admin role to a user via Keycloak Admin API
#
# Usage:
#   ./assign-platform-admin-role.sh <username>
#
# Environment variables:
#   KEYCLOAK_URL         - Keycloak base URL (default: https://keycloak.hypershell.localhost)
#   KEYCLOAK_REALM       - Realm name (default: hypershell)
#   KEYCLOAK_ADMIN_USER  - Admin username (default: admin)
#   KEYCLOAK_ADMIN_PASS  - Admin password (default: admin)

set -euo pipefail

USERNAME="${1:?Usage: $0 <username>}"
KEYCLOAK_URL="${KEYCLOAK_URL:-https://keycloak.hypershell.localhost}"
KEYCLOAK_REALM="${KEYCLOAK_REALM:-hypershell}"
KEYCLOAK_ADMIN_USER="${KEYCLOAK_ADMIN_USER:-admin}"
KEYCLOAK_ADMIN_PASS="${KEYCLOAK_ADMIN_PASS:-admin}"

echo "Assigning platform:admin role to user: ${USERNAME}"
echo ""

# Step 1: Get admin access token
echo "1. Acquiring admin access token..."
ADMIN_TOKEN=$(curl -sk -X POST \
  "${KEYCLOAK_URL}/realms/master/protocol/openid-connect/token" \
  -d "grant_type=password" \
  -d "client_id=admin-cli" \
  -d "username=${KEYCLOAK_ADMIN_USER}" \
  -d "password=${KEYCLOAK_ADMIN_PASS}" \
  | python3 -c "import json,sys; print(json.load(sys.stdin).get('access_token',''))" 2>/dev/null || true)

if [[ -z "$ADMIN_TOKEN" ]]; then
  echo "ERROR: Failed to acquire admin token"
  exit 1
fi
echo "✓ Admin token acquired"
echo ""

# Step 2: Get the user's ID
echo "2. Looking up user ID for: ${USERNAME}"
USER_ID=$(curl -sk -H "Authorization: Bearer ${ADMIN_TOKEN}" \
  "${KEYCLOAK_URL}/admin/realms/${KEYCLOAK_REALM}/users?username=${USERNAME}&exact=true" \
  | python3 -c "import json,sys; a=json.load(sys.stdin); print(a[0]['id'] if a else '')" 2>/dev/null || true)

if [[ -z "$USER_ID" ]]; then
  echo "ERROR: User '${USERNAME}' not found in realm '${KEYCLOAK_REALM}'"
  exit 1
fi
echo "✓ User ID: ${USER_ID}"
echo ""

# Step 3: Get the platform:admin role definition
echo "3. Looking up platform:admin role..."
ROLE_JSON=$(curl -sk -H "Authorization: Bearer ${ADMIN_TOKEN}" \
  "${KEYCLOAK_URL}/admin/realms/${KEYCLOAK_REALM}/roles/platform:admin" 2>/dev/null || true)

ROLE_ID=$(echo "$ROLE_JSON" | python3 -c "import json,sys; print(json.load(sys.stdin).get('id',''))" 2>/dev/null || true)
ROLE_NAME=$(echo "$ROLE_JSON" | python3 -c "import json,sys; print(json.load(sys.stdin).get('name',''))" 2>/dev/null || true)

if [[ -z "$ROLE_ID" || -z "$ROLE_NAME" ]]; then
  echo "ERROR: platform:admin role not found in realm '${KEYCLOAK_REALM}'"
  echo "You may need to create the role first via the Admin Console:"
  echo "  Realm roles → Create role → Name: platform:admin"
  exit 1
fi
echo "✓ Role ID: ${ROLE_ID}"
echo ""

# Step 4: Assign the role to the user
echo "4. Assigning platform:admin role to user..."
HTTP_CODE=$(curl -sk -o /dev/null -w '%{http_code}' -X POST \
  -H "Authorization: Bearer ${ADMIN_TOKEN}" \
  -H "Content-Type: application/json" \
  "${KEYCLOAK_URL}/admin/realms/${KEYCLOAK_REALM}/users/${USER_ID}/role-mappings/realm" \
  -d "[{\"id\":\"${ROLE_ID}\",\"name\":\"${ROLE_NAME}\"}]" 2>/dev/null || true)

if [[ "$HTTP_CODE" == "204" || "$HTTP_CODE" == "200" ]]; then
  echo "✓ Role assigned successfully (HTTP ${HTTP_CODE})"
else
  echo "ERROR: Role assignment failed (HTTP ${HTTP_CODE})"
  exit 1
fi
echo ""

# Step 5: Verify the assignment
echo "5. Verifying role assignment..."
ASSIGNED_ROLES=$(curl -sk -H "Authorization: Bearer ${ADMIN_TOKEN}" \
  "${KEYCLOAK_URL}/admin/realms/${KEYCLOAK_REALM}/users/${USER_ID}/role-mappings/realm" \
  2>/dev/null || true)

HAS_ROLE=$(echo "$ASSIGNED_ROLES" | python3 -c "
import json,sys
roles = json.load(sys.stdin)
has_it = any(r.get('name') == 'platform:admin' for r in roles)
print('true' if has_it else 'false')
" 2>/dev/null || echo "false")

if [[ "$HAS_ROLE" == "true" ]]; then
  echo "✓ Confirmed: User '${USERNAME}' has platform:admin role"
else
  echo "WARNING: Could not verify role assignment"
fi
echo ""

echo "SUCCESS! User '${USERNAME}' now has the platform:admin role."
echo ""
echo "Next steps:"
echo "  1. User should log out and log back in to get a fresh JWT"
echo "  2. New JWT will contain 'platform:admin' in realm_access.roles"
echo "  3. HyperShell API will sync the role and grant global gateway access"
