package users_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v4"
	. "github.com/onsi/gomega"

	"github.com/openshift-online/hypershell/components/api-server/pkg/api/openapi"
	"github.com/openshift-online/hypershell/components/api-server/pkg/rbac"
	"github.com/openshift-online/hypershell/components/api-server/plugins/roles"
	"github.com/openshift-online/hypershell/components/api-server/plugins/users"
	"github.com/openshift-online/hypershell/components/api-server/test"
	"github.com/openshift-online/rh-trex-ai/pkg/environments"
	"github.com/openshift-online/rh-trex-ai/pkg/testutil"
)

func jwtContextWithRealmRoles(h *test.Helper, account *testutil.TestAccount, realmRoles []string) context.Context {
	roleValues := make([]interface{}, len(realmRoles))
	for i, role := range realmRoles {
		roleValues[i] = role
	}

	claims := jwt.MapClaims{
		"iss":      h.Env().Config.APIClient.TokenURL,
		"username": account.Username,
		"typ":      "Bearer",
		"iat":      time.Now().Unix(),
		"exp":      time.Now().Add(time.Hour).Unix(),
		"realm_access": map[string]interface{}{
			"roles": roleValues,
		},
	}
	if account.Email != "" {
		claims["email"] = account.Email
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = testutil.JwkKID

	signedToken, err := token.SignedString(h.JWTPrivateKey)
	Expect(err).NotTo(HaveOccurred())

	return context.WithValue(context.Background(), openapi.ContextAccessToken, signedToken)
}

func seedUsers(count int) []string {
	userService := users.Service(&environments.Environment().Services)
	ids := make([]string, 0, count)
	for i := 0; i < count; i++ {
		username := fmt.Sprintf("registered-user-%d", i)
		id, err := userService.UpsertByUsername(context.Background(), username, nil, nil)
		Expect(err).NotTo(HaveOccurred())
		ids = append(ids, id)
	}
	return ids
}

func TestUserList_ForbiddenForGatewayCreator(t *testing.T) {
	h, client := test.RegisterIntegration(t)

	account := h.NewAccount("gateway-creator", "Gateway Creator", "creator@example.com")
	ctx := jwtContextWithRealmRoles(h, account, []string{roles.RoleGatewayCreator})

	_, resp, err := client.DefaultAPI.ListUsers(ctx).Execute()
	Expect(err).To(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(http.StatusForbidden))
}

func TestUserList_AllowedForHypershellAdmin(t *testing.T) {
	h, client := test.RegisterIntegration(t)

	seedUsers(2)
	account := h.NewAccount("dashboard-admin", "Dashboard Admin", "admin@example.com")
	ctx := jwtContextWithRealmRoles(h, account, []string{rbac.HypershellAdminRole})

	list, resp, err := client.DefaultAPI.ListUsers(ctx).Execute()
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(http.StatusOK))
	Expect(len(list.Items)).To(BeNumerically(">=", 2))
}

func TestUserList_AllowedForPlatformAdminBinding(t *testing.T) {
	h, client := test.RegisterIntegration(t)

	seedUsers(1)
	account := h.NewAccount("platform-admin", "Platform Admin", "platform@example.com")
	ctx := jwtContextWithRealmRoles(h, account, []string{roles.RolePlatformAdmin})

	list, resp, err := client.DefaultAPI.ListUsers(ctx).Execute()
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(http.StatusOK))
	Expect(*list.Total).To(BeNumerically(">=", 1))
}

func TestUserGet_Opaque404ForUnauthorizedCaller(t *testing.T) {
	h, client := test.RegisterIntegration(t)

	ids := seedUsers(1)
	account := h.NewAccount("unauthorized-viewer", "Unauthorized", "denied@example.com")
	ctx := jwtContextWithRealmRoles(h, account, []string{roles.RoleGatewayCreator})

	_, resp, err := client.DefaultAPI.GetUser(ctx, ids[0]).Execute()
	Expect(err).To(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(http.StatusNotFound))
}

func TestUserList_TotalAvailableWithSizeOne(t *testing.T) {
	h, client := test.RegisterIntegration(t)

	seedUsers(3)
	account := h.NewAccount("count-admin", "Count Admin", "count@example.com")
	ctx := jwtContextWithRealmRoles(h, account, []string{rbac.HypershellAdminRole})

	list, resp, err := client.DefaultAPI.ListUsers(ctx).Page(1).Size(1).OrderBy("username asc").Execute()
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(http.StatusOK))
	Expect(*list.Total).To(BeNumerically(">=", 3))
	Expect(len(list.Items)).To(Equal(1))
}

func TestUserGet_AllowedForAuthorizedCaller(t *testing.T) {
	h, client := test.RegisterIntegration(t)

	ids := seedUsers(1)
	account := h.NewAccount("get-admin", "Get Admin", "get@example.com")
	ctx := jwtContextWithRealmRoles(h, account, []string{rbac.HypershellAdminRole})

	user, resp, err := client.DefaultAPI.GetUser(ctx, ids[0]).Execute()
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(http.StatusOK))
	Expect(*user.Id).To(Equal(ids[0]))
	Expect(user.Username).NotTo(BeEmpty())
	Expect(user.CreatedAt).NotTo(BeNil())
}
