package rbac

import (
	"context"
	"strings"

	"github.com/openshift-online/rh-trex-ai/pkg/auth"
)

type UserUpserter interface {
	UpsertByUsername(ctx context.Context, username string, email *string, name *string) (userID string, err error)
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

	return p.upserter.UpsertByUsername(ctx, payload.Username, email, name)
}
