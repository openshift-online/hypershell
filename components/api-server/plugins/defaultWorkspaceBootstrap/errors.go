package defaultWorkspaceBootstrap

import "errors"

func errInvalidInput() error {
	return errors.New("gateway, endpoint, OIDC identity, subject, and user role are required")
}
