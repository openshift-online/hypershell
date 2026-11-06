package rbac

import (
	"context"
	"strings"

	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/auth"
)

type UserUpserter interface {
	UpsertByUsername(ctx context.Context, username string, email *string, name *string) (userID string, err error)
	// UpsertByUsernameWithSubject additionally captures the real Keycloak OIDC
	// subject (SEC-RBAC-03). users.UserService implements both.
	UpsertByUsernameWithSubject(ctx context.Context, username string, email *string, name *string, subject string) (userID string, err error)
}

type defaultUserProvisioner struct {
	upserter UserUpserter
}

var _ UserProvisioner = &defaultUserProvisioner{}

func NewUserProvisioner(upserter UserUpserter) UserProvisioner {
	return &defaultUserProvisioner{upserter: upserter}
}

func (p *defaultUserProvisioner) UpsertFromJWT(ctx context.Context, payload *auth.Payload) (string, error) {
	var email *string
	if payload.Email != "" {
		email = &payload.Email
	}

	var name *string
	fullName := strings.TrimSpace(payload.FirstName + " " + payload.LastName)
	if fullName != "" {
		name = &fullName
	}

	// auth.Payload (vendored from rh-trex-ai) carries no subject field, so the
	// JWT "sub" claim (SEC-RBAC-03) is read directly from the verified token
	// already stashed in ctx by the framework's auth middleware -- the same
	// extraction subjectFromGRPCContext uses for the gRPC path, which also
	// works unmodified here since both transports store the token under the
	// same context key (auth.ContextAuthKey / auth.TokenFromContext).
	subject := subjectFromGRPCContext(ctx)

	return p.upserter.UpsertByUsernameWithSubject(ctx, payload.Username, email, name, subject)
}
