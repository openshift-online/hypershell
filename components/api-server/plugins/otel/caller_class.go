package otel

import (
	"context"
	"os"
	"strings"

	"github.com/golang-jwt/jwt/v4"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/auth"
	"go.opentelemetry.io/otel/attribute"
)

// callerClassAttr classifies each HTTP request as synthetic (the platform's own
// probes, such as the outside prober) or user traffic, so the user API latency and
// user API availability indicators can count user traffic only (OBS-SLI-11). The
// value is bounded to exactly two values (synthetic, user); the raw azp, sub, or
// any other identity value is never recorded (API-OBS-06).
const callerClassAttr = attribute.Key("hypershell.caller.class")

const (
	callerClassSynthetic = "synthetic"
	callerClassUser      = "user"
)

// syntheticClientIDsEnv names the comma-separated list of OAuth client IDs whose
// verified tokens classify as synthetic (OBS-SLI-11). It is configuration, not
// code: an operator changes the list without a code change.
const syntheticClientIDsEnv = "HYPERSHELL_SYNTHETIC_CLIENT_IDS"

// defaultSyntheticClientID is the outside prober's Keycloak client
// (OBS-PRB-06), used when HYPERSHELL_SYNTHETIC_CLIENT_IDS is omitted.
const defaultSyntheticClientID = "hypershell-prober"

// syntheticClients is the set of OAuth client IDs (azp values) whose verified
// tokens classify as synthetic. A nil or empty set classifies every request as
// user.
type syntheticClients map[string]struct{}

// parseSyntheticClientIDs builds the synthetic client set from the raw
// HYPERSHELL_SYNTHETIC_CLIENT_IDS value (OBS-SLI-11). present reports whether the
// setting exists in the environment. When it is omitted the set is the prober's
// client; when it is present it is split on commas, each entry is trimmed, and
// empty entries are ignored, so an empty value yields an empty set.
func parseSyntheticClientIDs(value string, present bool) syntheticClients {
	if !present {
		return syntheticClients{defaultSyntheticClientID: {}}
	}
	clients := syntheticClients{}
	for _, entry := range strings.Split(value, ",") {
		if id := strings.TrimSpace(entry); id != "" {
			clients[id] = struct{}{}
		}
	}
	return clients
}

// loadSyntheticClients reads HYPERSHELL_SYNTHETIC_CLIENT_IDS. The plugin calls it
// once at startup and passes the result to the HTTP instrumentation, so the list
// is not re-read per request.
func loadSyntheticClients() syntheticClients {
	value, present := os.LookupEnv(syntheticClientIDsEnv)
	return parseSyntheticClientIDs(value, present)
}

// classifyCaller maps verified token claims to the bounded caller class
// (OBS-SLI-11). It returns synthetic only when the azp claim is a string equal to
// an entry of the synthetic client set; a nil claim set, a missing or non-string
// azp, an unlisted azp, and an empty set all return user, so user traffic is never
// hidden by mistake. It is a pure function so the classification is unit-testable
// without a running server. The caller MUST pass only claims of a token that the
// API server verified.
func classifyCaller(claims jwt.MapClaims, clients syntheticClients) string {
	if claims == nil || len(clients) == 0 {
		return callerClassUser
	}
	azp, ok := claims["azp"].(string)
	if !ok || azp == "" {
		return callerClassUser
	}
	if _, listed := clients[azp]; listed {
		return callerClassSynthetic
	}
	return callerClassUser
}

// callerClassFromContext classifies the request from the token that the
// framework's JWT handler verified and stored in the request context
// (auth.TokenFromContext). The JWT handler wraps the whole pre-auth chain, so the
// otelhttp handler sees the verified token. Headers, the user agent, the source
// address, and unverified claims are never read: a request with no verified token
// (no token, a public path, or a token the handler did not accept) is user. The
// token.Valid check rejects any token that reached the context without signature
// verification (jwt.Parse sets Valid only after verifying it).
func callerClassFromContext(ctx context.Context, clients syntheticClients) string {
	token, err := auth.TokenFromContext(ctx)
	if err != nil || token == nil || !token.Valid {
		return callerClassUser
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return callerClassUser
	}
	return classifyCaller(claims, clients)
}
