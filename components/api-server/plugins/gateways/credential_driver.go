package gateways

import (
	"encoding/json"
	"reflect"
	"strings"

	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/errors"
)

// credentialDriverImmutableReason is the reason given when an update would change
// a gateway's credential driver (openshell-gateway-credentials.spec.md OSG-CRED-13).
// Provider credentials live in the driver's backend, so switching drivers would
// orphan them.
const credentialDriverImmutableReason = "credential driver cannot be changed after the gateway is created; " +
	"provider credentials stored in the current driver would be orphaned. " +
	"Migrate credentials manually, then delete and re-create the gateway with the desired driver"

// enforceGatewayCredentialDriverUpdateSemantics keeps a gateway's effective
// credential driver fixed after creation, for every writer class, the control
// plane included. A user or import change is already rejected earlier by the
// field policy; this check covers the control-plane write path.
//
// Comparison is by effective driver, not stored bytes: NULL, "", "{}" and
// {"type":"vault"} all mean the default vault driver, so re-sending any of them
// on a vault gateway is a no-op. A next value of nil or "" leaves the stored
// value untouched (patch semantics).
func enforceGatewayCredentialDriverUpdateSemantics(current, next *Gateway) *errors.ServiceError {
	if next.CredentialDriver == nil || strings.TrimSpace(*next.CredentialDriver) == "" {
		next.CredentialDriver = current.CredentialDriver
		return nil
	}
	if strEq(current.CredentialDriver, next.CredentialDriver) {
		return nil
	}
	nextDriver, err := effectiveCredentialDriver(next.CredentialDriver)
	if err != nil {
		return errors.Validation("credential_driver is not a valid JSON object: %s", err)
	}
	currentDriver, err := effectiveCredentialDriver(current.CredentialDriver)
	if err != nil {
		// The stored value cannot be parsed, so no change can be proven safe.
		// Only the byte-identical resend accepted above is allowed.
		return errors.Conflict("%s", credentialDriverImmutableReason)
	}
	if !reflect.DeepEqual(currentDriver, nextDriver) {
		return errors.Conflict("%s", credentialDriverImmutableReason)
	}
	// Equivalent resend: keep the stored bytes so the row does not change and
	// generation is not bumped by a spelling difference (NULL vs {"type":"vault"}).
	next.CredentialDriver = current.CredentialDriver
	return nil
}

// effectiveCredentialDriver parses a stored or incoming credential_driver into a
// generic object and applies the default: an empty or missing type means vault.
func effectiveCredentialDriver(raw *string) (map[string]interface{}, error) {
	driver := map[string]interface{}{}
	if raw != nil && strings.TrimSpace(*raw) != "" && strings.TrimSpace(*raw) != "null" {
		if err := json.Unmarshal([]byte(*raw), &driver); err != nil {
			return nil, err
		}
		if driver == nil {
			driver = map[string]interface{}{}
		}
	}
	if t, _ := driver["type"].(string); strings.TrimSpace(t) == "" {
		driver["type"] = "vault"
	}
	return driver, nil
}
