package otel

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v4"
	"github.com/gorilla/mux"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/audit"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/auth"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/config"
	pkgenv "github.com/openshift-online/rh-trex-ai/components/api-server/pkg/environments"
	pkgserver "github.com/openshift-online/rh-trex-ai/components/api-server/pkg/server"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/trex"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

const (
	proberClientID   = "hypershell-prober"
	frontendClientID = "hypershell-frontend"
)

// TestClassifyCaller verifies the OBS-SLI-11 classification table: synthetic only
// for a string azp in the configured list; user for an unlisted, missing,
// non-string, or empty azp, for nil claims, and for an empty list
// (OBS-SLI-11.01, .02, .05, .07).
func TestClassifyCaller(t *testing.T) {
	prober := syntheticClients{proberClientID: {}}
	tests := []struct {
		name    string
		claims  jwt.MapClaims
		clients syntheticClients
		want    string
	}{
		{name: "listed azp is synthetic", claims: jwt.MapClaims{"azp": proberClientID}, clients: prober, want: callerClassSynthetic},
		{name: "listed azp among several is synthetic", claims: jwt.MapClaims{"azp": "other-probe"}, clients: syntheticClients{proberClientID: {}, "other-probe": {}}, want: callerClassSynthetic},
		{name: "unlisted azp is user", claims: jwt.MapClaims{"azp": frontendClientID}, clients: prober, want: callerClassUser},
		{name: "missing azp is user", claims: jwt.MapClaims{"sub": "abc"}, clients: prober, want: callerClassUser},
		{name: "non-string azp is user", claims: jwt.MapClaims{"azp": []interface{}{proberClientID}}, clients: prober, want: callerClassUser},
		{name: "numeric azp is user", claims: jwt.MapClaims{"azp": 42}, clients: prober, want: callerClassUser},
		{name: "empty azp is user", claims: jwt.MapClaims{"azp": ""}, clients: syntheticClients{"": {}}, want: callerClassUser},
		{name: "azp differing only in case is user", claims: jwt.MapClaims{"azp": "Hypershell-Prober"}, clients: prober, want: callerClassUser},
		{name: "nil claims are user", claims: nil, clients: prober, want: callerClassUser},
		{name: "empty list classifies the prober as user", claims: jwt.MapClaims{"azp": proberClientID}, clients: syntheticClients{}, want: callerClassUser},
		{name: "nil list classifies the prober as user", claims: jwt.MapClaims{"azp": proberClientID}, clients: nil, want: callerClassUser},
		{name: "sub equal to a listed client is user", claims: jwt.MapClaims{"sub": proberClientID}, clients: prober, want: callerClassUser},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := classifyCaller(tc.claims, tc.clients)
			if got != tc.want {
				t.Fatalf("classifyCaller(%v) = %q, want %q", tc.claims, got, tc.want)
			}
			if got != callerClassSynthetic && got != callerClassUser {
				t.Fatalf("classifyCaller returned %q, not one of the two bounded values", got)
			}
		})
	}
}

// TestParseSyntheticClientIDs verifies the HYPERSHELL_SYNTHETIC_CLIENT_IDS
// contract (OBS-SLI-11): omitted means the prober client, an empty value means no
// clients, entries are trimmed, and empty entries are ignored (OBS-SLI-11.07,
// OBS-SLI-11.08).
func TestParseSyntheticClientIDs(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		present bool
		want    []string
	}{
		{name: "omitted defaults to the prober client", present: false, want: []string{proberClientID}},
		{name: "empty value is an empty list", value: "", present: true, want: nil},
		{name: "only separators and spaces is an empty list", value: " , ,, ", present: true, want: nil},
		{name: "single entry", value: proberClientID, present: true, want: []string{proberClientID}},
		{name: "entries are trimmed and empty entries ignored", value: " hypershell-prober , ,other-probe ,", present: true, want: []string{proberClientID, "other-probe"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := parseSyntheticClientIDs(tc.value, tc.present)
			if len(got) != len(tc.want) {
				t.Fatalf("parseSyntheticClientIDs(%q, %v) = %v, want %v", tc.value, tc.present, got, tc.want)
			}
			for _, id := range tc.want {
				if _, ok := got[id]; !ok {
					t.Fatalf("parseSyntheticClientIDs(%q, %v) = %v, missing %q", tc.value, tc.present, got, id)
				}
			}
		})
	}
}

// TestLoadSyntheticClientsReadsEnvironment verifies that the startup loader
// distinguishes an omitted setting (default prober client) from an empty one
// (empty list), per OBS-SLI-11.07 and OBS-SLI-11.08.
func TestLoadSyntheticClientsReadsEnvironment(t *testing.T) {
	t.Run("omitted", func(t *testing.T) {
		t.Setenv(syntheticClientIDsEnv, "")
		if err := os.Unsetenv(syntheticClientIDsEnv); err != nil {
			t.Fatal(err)
		}
		got := loadSyntheticClients()
		if _, ok := got[proberClientID]; !ok || len(got) != 1 {
			t.Fatalf("omitted %s gave %v, want only %q", syntheticClientIDsEnv, got, proberClientID)
		}
	})
	t.Run("empty", func(t *testing.T) {
		t.Setenv(syntheticClientIDsEnv, "")
		if got := loadSyntheticClients(); len(got) != 0 {
			t.Fatalf("empty %s gave %v, want an empty list", syntheticClientIDsEnv, got)
		}
	})
	t.Run("set", func(t *testing.T) {
		t.Setenv(syntheticClientIDsEnv, "probe-a,probe-b")
		got := loadSyntheticClients()
		if _, ok := got["probe-a"]; !ok || len(got) != 2 {
			t.Fatalf("set %s gave %v, want probe-a and probe-b", syntheticClientIDsEnv, got)
		}
	})
}

// verifiedToken returns a token as the framework's JWT handler stores it after
// verification: parsed claims with Valid set (jwt.Parse sets Valid only after the
// signature and standard claims check out).
func verifiedToken(claims jwt.MapClaims) *jwt.Token {
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Valid = true
	return token
}

// TestCallerClassFromContext verifies that the classification reads only a
// verified token from the request context: no token, a token that was never
// verified (Valid false, as jwt.ParseUnverified leaves it), and non-map claims are
// all user (OBS-SLI-11.03, OBS-SLI-11.04).
func TestCallerClassFromContext(t *testing.T) {
	clients := syntheticClients{proberClientID: {}}

	unverified := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{"azp": proberClientID})
	structClaims := jwt.NewWithClaims(jwt.SigningMethodRS256, &jwt.RegisteredClaims{Subject: proberClientID})
	structClaims.Valid = true

	tests := []struct {
		name string
		ctx  context.Context
		want string
	}{
		{name: "verified prober token is synthetic", ctx: context.WithValue(context.Background(), auth.ContextAuthKey, verifiedToken(jwt.MapClaims{"azp": proberClientID})), want: callerClassSynthetic},
		{name: "verified user token is user", ctx: context.WithValue(context.Background(), auth.ContextAuthKey, verifiedToken(jwt.MapClaims{"azp": frontendClientID})), want: callerClassUser},
		{name: "no token is user", ctx: context.Background(), want: callerClassUser},
		{name: "unverified prober token is user", ctx: context.WithValue(context.Background(), auth.ContextAuthKey, unverified), want: callerClassUser},
		{name: "non-map claims are user", ctx: context.WithValue(context.Background(), auth.ContextAuthKey, structClaims), want: callerClassUser},
		{name: "wrong context value type is user", ctx: context.WithValue(context.Background(), auth.ContextAuthKey, "hypershell-prober"), want: callerClassUser},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := callerClassFromContext(tc.ctx, clients); got != tc.want {
				t.Fatalf("callerClassFromContext = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestOTelHTTPHandlerRecordsCallerClass serves requests through the production
// HTTP instrumentation with an in-memory metric reader. A stand-in for the
// framework's JWT handler stores the verified token in the context, as the real
// handler does (see TestCallerClassBehindFrameworkJWTHandler for the real one).
// It checks the caller class on the recorded latency sample for verified prober
// and user tokens, no token, unreadable or unlisted azp claims, spoofed headers,
// an empty and an omitted synthetic client list, and an unmatched route. It also
// checks that the attribute complements the request-class attribute and that no
// identity value is recorded.
// Verifies: OBS-SLI-11.01, OBS-SLI-11.02, OBS-SLI-11.03, OBS-SLI-11.05, OBS-SLI-11.06, OBS-SLI-11.07, OBS-SLI-11.08
func TestOTelHTTPHandlerRecordsCallerClass(t *testing.T) {
	omitted := parseSyntheticClientIDs("", false)
	tests := []struct {
		name    string
		path    string
		token   *jwt.Token
		headers map[string]string
		// clients is the startup synthetic client list; nil means the setting
		// is omitted (the hypershell-prober default).
		clients syntheticClients
		want    string
	}{
		{
			name:  "verified prober token with the omitted setting records synthetic",
			path:  "/api/hypershell/v1/gateways/abc123",
			token: verifiedToken(jwt.MapClaims{"azp": proberClientID, "sub": "prober-sub-123"}),
			want:  callerClassSynthetic,
		},
		{
			name:  "verified user token records user",
			path:  "/api/hypershell/v1/gateways/abc123",
			token: verifiedToken(jwt.MapClaims{"azp": frontendClientID, "sub": "user-sub-456"}),
			want:  callerClassUser,
		},
		{
			name: "unauthenticated request records user",
			path: "/api/hypershell/v1/gateways/abc123",
			want: callerClassUser,
		},
		{
			name:  "forged headers with a user token record user",
			path:  "/api/hypershell/v1/gateways/abc123",
			token: verifiedToken(jwt.MapClaims{"azp": frontendClientID, "sub": "user-sub-456"}),
			headers: map[string]string{
				"User-Agent":                 proberClientID,
				"X-Hypershell-Caller-Class":  callerClassSynthetic,
				"X-Synthetic":                "true",
				"X-Forwarded-Client-Id":      proberClientID,
				"X-Forwarded-Access-Token":   "forged",
				"X-Hypershell-Synthetic-Azp": proberClientID,
			},
			want: callerClassUser,
		},
		{
			name: "forged headers without a token record user",
			path: "/api/hypershell/v1/gateways/abc123",
			headers: map[string]string{
				"User-Agent":                proberClientID,
				"X-Hypershell-Caller-Class": callerClassSynthetic,
			},
			want: callerClassUser,
		},
		{
			name:  "verified token without azp records user",
			path:  "/api/hypershell/v1/gateways/abc123",
			token: verifiedToken(jwt.MapClaims{"sub": proberClientID}),
			want:  callerClassUser,
		},
		{
			name:  "verified token with a non-string azp records user",
			path:  "/api/hypershell/v1/gateways/abc123",
			token: verifiedToken(jwt.MapClaims{"azp": []interface{}{proberClientID}}),
			want:  callerClassUser,
		},
		{
			name:    "empty synthetic client list records the prober as user",
			path:    "/api/hypershell/v1/gateways/abc123",
			token:   verifiedToken(jwt.MapClaims{"azp": proberClientID}),
			clients: parseSyntheticClientIDs("", true),
			want:    callerClassUser,
		},
		{
			name:    "configured list replaces the default",
			path:    "/api/hypershell/v1/gateways/abc123",
			token:   verifiedToken(jwt.MapClaims{"azp": proberClientID}),
			clients: parseSyntheticClientIDs("other-probe", true),
			want:    callerClassUser,
		},
		{
			name:  "unmatched route still records the caller class",
			path:  "/api/hypershell/v1/does-not-exist",
			token: verifiedToken(jwt.MapClaims{"azp": proberClientID}),
			want:  callerClassSynthetic,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			reader := installManualReader(t)

			router := mux.NewRouter()
			router.Use(routeTagMiddleware)
			router.HandleFunc("/api/hypershell/v1/gateways/{id}", func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			}).Methods(http.MethodGet)

			clients := tc.clients
			if clients == nil {
				clients = omitted
			}
			instrumented := newOTelHTTPHandler(clients)(router)
			// Stand-in for the framework JWT handler, which wraps the pre-auth
			// chain and stores the verified token in the context.
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tc.token != nil {
					r = r.WithContext(context.WithValue(r.Context(), auth.ContextAuthKey, tc.token))
				}
				instrumented.ServeHTTP(w, r)
			})

			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			for k, v := range tc.headers {
				req.Header.Set(k, v)
			}
			handler.ServeHTTP(httptest.NewRecorder(), req)

			attrs := durationSampleAttrs(t, reader)
			class, ok := attrs.Value(callerClassAttr)
			if !ok {
				t.Fatalf("latency sample carries no %q attribute", callerClassAttr)
			}
			if class.AsString() != tc.want {
				t.Fatalf("%s = %q, want %q", callerClassAttr, class.AsString(), tc.want)
			}
			// Complements, never replaces, the OBS-SLI-08 request class.
			if rc, ok := attrs.Value(requestClassAttr); !ok || rc.AsString() != requestClassRead {
				t.Fatalf("latency sample lost %q (got %v); the caller class must complement it", requestClassAttr, rc)
			}
			assertNoIdentityValues(t, attrs, tc.token)
		})
	}
}

// installManualReader installs a fresh in-memory meter provider as the global
// provider for the test and returns its reader.
func installManualReader(t *testing.T) *sdkmetric.ManualReader {
	t.Helper()
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	prev := otel.GetMeterProvider()
	otel.SetMeterProvider(mp)
	t.Cleanup(func() { otel.SetMeterProvider(prev) })
	return reader
}

// assertNoIdentityValues fails if any recorded attribute value equals an azp or
// sub claim of the token (OBS-SLI-11, API-OBS-06).
func assertNoIdentityValues(t *testing.T, attrs attribute.Set, token *jwt.Token) {
	t.Helper()
	if token == nil {
		return
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return
	}
	for _, key := range []string{"azp", "sub"} {
		v, ok := claims[key].(string)
		if !ok || v == "" {
			continue
		}
		for _, kv := range attrs.ToSlice() {
			if kv.Value.Type() == attribute.STRING && kv.Value.AsString() == v {
				t.Fatalf("latency sample attribute %s carries the %s value %q", kv.Key, key, v)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Real framework wiring: the JWT handler must run before the otel handler.
// ---------------------------------------------------------------------------

const (
	testBasePath = "/api/hypershell/v1"
	testIssuer   = "https://keycloak.test/realms/hypershell"
	testKID      = "caller-class-test-kid"
)

var registerOTelPreAuthOnce sync.Once

// TestCallerClassBehindFrameworkJWTHandler builds the framework's real HTTP server
// (pkgserver.NewDefaultAPIServer) with JWT enabled and the otel handler registered
// as a pre-auth middleware, exactly as plugin.go registers it. It confirms that
// the framework runs this middleware after the JWT handler, so the verified token
// is what the classification reads (OBS-SLI-11.01, .02, .03, .04):
//   - a verified prober token on an authenticated path records synthetic;
//   - a verified user token records user;
//   - a prober token with a forged signature is refused with 401 and records no
//     sample;
//   - a forged prober token on a public path (where the JWT handler does not
//     verify tokens) records user, because only verified claims count;
//   - a request with no token records user.
//
// Verifies: OBS-SLI-11.01, OBS-SLI-11.02, OBS-SLI-11.03, OBS-SLI-11.04
func TestCallerClassBehindFrameworkJWTHandler(t *testing.T) {
	if os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") != "" {
		// plugin.go init() has already registered the production otel pre-auth
		// middleware, so a second registration here would count every request
		// twice.
		t.Skip("OTEL_EXPORTER_OTLP_ENDPOINT is set; plugin init() already registered the otel pre-auth middleware")
	}
	trex.Init(trex.Config{ServiceName: "hypershell", BasePath: testBasePath})

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generating RSA key: %v", err)
	}
	forger, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generating forger RSA key: %v", err)
	}
	jwkFile := writeJWKSet(t, testKID, &key.PublicKey)

	registerOTelPreAuthOnce.Do(func() {
		pkgserver.RegisterPreAuthMiddleware(newOTelHTTPHandler(parseSyntheticClientIDs("", false)))
	})

	reader := installManualReader(t)

	cfg := config.NewApplicationConfig()
	cfg.Auth.EnableJWT = true
	cfg.Auth.JwkCertFile = jwkFile
	cfg.Auth.JwkCertURLs = nil
	cfg.Auth.JWTIssuer = testIssuer
	cfg.Auth.JWTAudience = ""
	cfg.Server.ACLFile = ""
	cfg.Server.BindAddress = "127.0.0.1:0"
	env := &pkgenv.Env{Config: cfg, Audit: audit.New(nil, nil)}

	server := pkgserver.NewDefaultAPIServer(env, []byte(`{}`))
	listener, err := server.Listen()
	if err != nil {
		t.Fatalf("listening: %v", err)
	}
	go server.Serve(listener)
	t.Cleanup(func() {
		if err := server.Stop(); err != nil {
			t.Errorf("stopping server: %v", err)
		}
	})
	base := "http://" + listener.Addr().String()
	client := &http.Client{Timeout: 5 * time.Second}

	sign := func(signer *rsa.PrivateKey, azp string) string {
		t.Helper()
		token := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
			"iss": testIssuer,
			"azp": azp,
			"sub": "sub-" + azp,
			"iat": time.Now().Unix(),
			"exp": time.Now().Add(time.Hour).Unix(),
		})
		token.Header["kid"] = testKID
		signed, err := token.SignedString(signer)
		if err != nil {
			t.Fatalf("signing token: %v", err)
		}
		return signed
	}

	// Authenticated (non-public) path that matches no registered route: the JWT
	// handler verifies the token, then the router answers 404, with no database.
	const authenticatedPath = testBasePath + "/caller-class-probe"
	const publicPath = testBasePath + "/openapi"

	tests := []struct {
		name       string
		path       string
		bearer     string
		wantStatus int
		wantDelta  map[string]uint64
	}{
		{
			name:       "verified prober token records synthetic",
			path:       authenticatedPath,
			bearer:     sign(key, proberClientID),
			wantStatus: http.StatusNotFound,
			wantDelta:  map[string]uint64{callerClassSynthetic: 1},
		},
		{
			name:       "verified user token records user",
			path:       authenticatedPath,
			bearer:     sign(key, frontendClientID),
			wantStatus: http.StatusNotFound,
			wantDelta:  map[string]uint64{callerClassUser: 1},
		},
		{
			name:       "forged prober token is refused and records nothing",
			path:       authenticatedPath,
			bearer:     sign(forger, proberClientID),
			wantStatus: http.StatusUnauthorized,
			wantDelta:  map[string]uint64{},
		},
		{
			name:       "forged prober token on a public path records user",
			path:       publicPath,
			bearer:     sign(forger, proberClientID),
			wantStatus: http.StatusOK,
			wantDelta:  map[string]uint64{callerClassUser: 1},
		},
		{
			name:       "no token on a public path records user",
			path:       publicPath,
			wantStatus: http.StatusOK,
			wantDelta:  map[string]uint64{callerClassUser: 1},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			before := callerClassCounts(t, reader)

			req, err := http.NewRequest(http.MethodGet, base+tc.path, nil)
			if err != nil {
				t.Fatal(err)
			}
			if tc.bearer != "" {
				req.Header.Set("Authorization", "Bearer "+tc.bearer)
			}
			req.Header.Set("User-Agent", proberClientID)
			req.Header.Set("X-Hypershell-Caller-Class", callerClassSynthetic)
			resp, err := client.Do(req)
			if err != nil {
				t.Fatalf("request: %v", err)
			}
			_ = resp.Body.Close()
			if resp.StatusCode != tc.wantStatus {
				t.Fatalf("status = %d, want %d", resp.StatusCode, tc.wantStatus)
			}

			after := callerClassCounts(t, reader)
			for _, class := range []string{callerClassSynthetic, callerClassUser} {
				if got := after[class] - before[class]; got != tc.wantDelta[class] {
					t.Fatalf("%s=%s samples grew by %d, want %d (before %v, after %v)",
						callerClassAttr, class, got, tc.wantDelta[class], before, after)
				}
			}
			for class := range after {
				if class != callerClassSynthetic && class != callerClassUser {
					t.Fatalf("unexpected %s value %q", callerClassAttr, class)
				}
			}
		})
	}
}

// callerClassCounts returns the cumulative http.server.request.duration sample
// count per caller class. A sample without the attribute is counted under "".
func callerClassCounts(t *testing.T, reader *sdkmetric.ManualReader) map[string]uint64 {
	t.Helper()
	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("collecting metrics: %v", err)
	}
	counts := map[string]uint64{}
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != httpServerRequestDurationName {
				continue
			}
			hist, ok := m.Data.(metricdata.Histogram[float64])
			if !ok {
				t.Fatalf("instrument %q is %T, want Histogram[float64]", m.Name, m.Data)
			}
			for _, dp := range hist.DataPoints {
				class := ""
				if v, ok := dp.Attributes.Value(callerClassAttr); ok {
					class = v.AsString()
				}
				counts[class] += dp.Count
			}
		}
	}
	return counts
}

// writeJWKSet writes a one-key JWK set file that the framework JWT handler loads.
func writeJWKSet(t *testing.T, kid string, pub *rsa.PublicKey) string {
	t.Helper()
	set := map[string]interface{}{
		"keys": []map[string]string{{
			"kty": "RSA",
			"kid": kid,
			"use": "sig",
			"alg": "RS256",
			"n":   base64.RawURLEncoding.EncodeToString(pub.N.Bytes()),
			"e":   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pub.E)).Bytes()),
		}},
	}
	data, err := json.Marshal(set)
	if err != nil {
		t.Fatalf("encoding JWK set: %v", err)
	}
	path := filepath.Join(t.TempDir(), "jwks.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("writing JWK set: %v", err)
	}
	return path
}
