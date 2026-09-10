package users_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v4"
	. "github.com/onsi/gomega"

	"github.com/openshift-online/hypershell/components/api-server/pkg/api"
	"github.com/openshift-online/hypershell/components/api-server/pkg/api/openapi"
	"github.com/openshift-online/hypershell/components/api-server/pkg/rbac"
	"github.com/openshift-online/hypershell/components/api-server/plugins/roleBindings"
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

func TestUserActivityStats_AllowedForHypershellAdmin(t *testing.T) {
	h, client := test.RegisterIntegration(t)

	seedUsers(3)
	account := h.NewAccount("stats-admin", "Stats Admin", "stats@example.com")
	ctx := jwtContextWithRealmRoles(h, account, []string{rbac.HypershellAdminRole})

	stats, resp, err := client.DefaultAPI.GetUserActivityStats(ctx).Execute()
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(http.StatusOK))
	Expect(stats.TotalRegistered).To(BeNumerically(">=", 3))
	Expect(len(stats.RegistrationDaily)).To(Equal(30))
	Expect(len(stats.ActiveDaily)).To(Equal(30))
}

func TestUserActivityStats_ForbiddenForGatewayCreator(t *testing.T) {
	h, client := test.RegisterIntegration(t)

	account := h.NewAccount("stats-creator", "Stats Creator", "creator@example.com")
	ctx := jwtContextWithRealmRoles(h, account, []string{roles.RoleGatewayCreator})

	_, resp, err := client.DefaultAPI.GetUserActivityStats(ctx).Execute()
	Expect(err).To(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(http.StatusForbidden))
}

func TestUserActivityStats_ForbiddenForGatewayCreatorBinding(t *testing.T) {
	h, client := test.RegisterIntegration(t)

	account := h.NewAccount("stats-bound-creator", "Bound Creator", "bound-creator@example.com")
	ctx := h.NewAuthenticatedContext(account)

	userService := users.Service(&environments.Environment().Services)
	userID, userErr := userService.UpsertByUsername(context.Background(), account.Username, nil, nil)
	Expect(userErr).NotTo(HaveOccurred())

	roleService := roles.Service(&environments.Environment().Services)
	creatorRole, roleErr := roleService.GetByName(context.Background(), roles.RoleGatewayCreator)
	Expect(roleErr).NotTo(HaveOccurred())

	rbDao := roleBindings.NewRoleBindingDao(&environments.Environment().Database.SessionFactory)
	_, bindErr := rbDao.Create(context.Background(), &roleBindings.RoleBinding{
		RoleID: creatorRole.ID,
		Scope:  roleBindings.ScopeGlobal,
		UserID: &userID,
	})
	Expect(bindErr).NotTo(HaveOccurred())

	_, resp, err := client.DefaultAPI.GetUserActivityStats(ctx).Execute()
	Expect(err).To(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(http.StatusForbidden))
}

func seedUserWithCreatedAt(username string, createdAt time.Time) {
	env := environments.Environment()
	g2 := env.Database.SessionFactory.New(context.Background())
	now := time.Now().UTC()
	Expect(
		g2.Exec(
			`INSERT INTO users (id, username, created_at, updated_at) VALUES (?, ?, ?, ?)`,
			api.NewID(),
			username,
			createdAt,
			now,
		).Error,
	).NotTo(HaveOccurred())
}

func TestUserActivityStats_RegistrationWindowBoundaries(t *testing.T) {
	_, _ = test.RegisterIntegration(t)

	evaluationTime := time.Date(2026, 9, 8, 15, 30, 0, 0, time.UTC)
	endDay := time.Date(2026, 9, 8, 0, 0, 0, 0, time.UTC)
	last7DayStart := endDay.AddDate(0, 0, -6)
	last30DayStart := endDay.AddDate(0, 0, -29)

	seedUserWithCreatedAt("boundary-7-in", last7DayStart)
	seedUserWithCreatedAt("boundary-7-out", last7DayStart.Add(-time.Nanosecond))
	seedUserWithCreatedAt("boundary-30-in", last30DayStart)
	seedUserWithCreatedAt("boundary-30-out", last30DayStart.Add(-time.Nanosecond))

	dao := users.NewUserDao(&environments.Environment().Database.SessionFactory)
	stats, err := dao.GetActivityStats(context.Background(), evaluationTime)
	Expect(err).NotTo(HaveOccurred())
	Expect(stats.TotalRegistered).To(Equal(int64(4)))
	Expect(stats.RegisteredLast7Days).To(Equal(int64(1)))
	Expect(stats.RegisteredLast30Days).To(Equal(int64(3)))
}

func TestUserActivityStats_RecordsLoginOnProvisioning(t *testing.T) {
	h, client := test.RegisterIntegration(t)

	account := h.NewAccount("login-tracker", "Login Tracker", "login@example.com")
	ctx := jwtContextWithRealmRoles(h, account, []string{rbac.HypershellAdminRole})

	_, resp, err := client.DefaultAPI.ListUsers(ctx).Page(1).Size(1).Execute()
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(http.StatusOK))

	stats, statsResp, statsErr := client.DefaultAPI.GetUserActivityStats(ctx).Execute()
	Expect(statsErr).NotTo(HaveOccurred())
	Expect(statsResp.StatusCode).To(Equal(http.StatusOK))
	Expect(stats.ActiveLast7Days).To(BeNumerically(">=", 1))
}
