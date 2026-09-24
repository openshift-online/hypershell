package test

import (
	"encoding/json"
	stderrors "errors"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v4"

	"github.com/openshift-online/hypershell/components/api-server/pkg/api/openapi"

	"github.com/openshift-online/rh-trex-ai/pkg/testutil"
)

// CreateJWTStringWithClaims signs a test JWT for account like CreateJWTString,
// then applies extra claims on top (for example "sub" or "realm_access"). It is
// used to act as an OIDC client_credentials caller, such as a control plane
// whose JWT subject is the key of its ManagedCluster registration.
func (helper *Helper) CreateJWTStringWithClaims(account *testutil.TestAccount, extra jwt.MapClaims) string {
	claims := jwt.MapClaims{
		"iss":        helper.AppConfig.APIClient.TokenURL,
		"username":   strings.ToLower(account.Username),
		"first_name": account.FirstName,
		"last_name":  account.LastName,
		"typ":        "Bearer",
		"iat":        time.Now().Unix(),
		"exp":        time.Now().Add(1 * time.Hour).Unix(),
	}
	if account.Email != "" {
		claims["email"] = account.Email
	}
	for k, v := range extra {
		claims[k] = v
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = testutil.JwkKID

	signed, err := token.SignedString(helper.JWTPrivateKey)
	if err != nil {
		helper.T.Errorf("Unable to sign test jwt: %s", err)
		return ""
	}
	return signed
}

// ControlPlaneClaims are the extra claims of a control plane's
// client_credentials token: its unique subject and the
// managed-cluster-registrar realm role.
func ControlPlaneClaims(subject string) jwt.MapClaims {
	return jwt.MapClaims{
		"sub":          subject,
		"realm_access": map[string]interface{}{"roles": []interface{}{"managed-cluster-registrar"}},
	}
}

// APIErrorReason returns the "reason" of an API error returned by the generated
// client, or "" when err is not an API error with a JSON Error body.
func APIErrorReason(err error) string {
	var apiErr *openapi.GenericOpenAPIError
	if !stderrors.As(err, &apiErr) {
		return ""
	}
	var body openapi.Error
	if jsonErr := json.Unmarshal(apiErr.Body(), &body); jsonErr != nil {
		return ""
	}
	return body.GetReason()
}
