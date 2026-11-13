// Package openshellclient is a thin client over the OpenShell gateway's existing
// gRPC provisioning API (openshell.v1.OpenShell), used by the control plane to
// realize the gateway-side workspace and memberships a CITrust needs
// (PLAT-CITR-15). It consumes OpenShell's existing operations (the ones the
// `openshell` CLI drives) rather than defining a new contract, consistent with
// agent-workspace-gateway-api.spec.md (AGT-WSP). The protos are vendored from the
// exact rhaiv build the gateway runs (opendatahub-io/openshell at the pinned tag).
package openshellclient

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	datamodelv1 "github.com/openshift-online/hypershell/components/control-plane/internal/openshellclient/gen/datamodelv1"
	openshellv1 "github.com/openshift-online/hypershell/components/control-plane/internal/openshellclient/gen/openshellv1"
)

// Role is the workspace role granted to a member.
type Role int

const (
	// RoleUser maps to OpenShell WORKSPACE_ROLE_USER.
	RoleUser Role = iota
	// RoleAdmin maps to OpenShell WORKSPACE_ROLE_ADMIN.
	RoleAdmin
)

// RoleFromGatewayRole maps a HyperShell gateway client role ("openshell-admin" /
// "openshell-user") to the OpenShell workspace role. Anything other than the admin
// role is treated as the least-privilege user role.
func RoleFromGatewayRole(gatewayRole string) Role {
	if strings.EqualFold(gatewayRole, "openshell-admin") {
		return RoleAdmin
	}
	return RoleUser
}

func (r Role) proto() openshellv1.WorkspaceRole {
	if r == RoleAdmin {
		return openshellv1.WorkspaceRole_WORKSPACE_ROLE_ADMIN
	}
	return openshellv1.WorkspaceRole_WORKSPACE_ROLE_USER
}

// Session is a short-lived admin session to one gateway's OpenShell provisioning
// API. Every call carries the admin bearer token in gRPC metadata. Close it when
// the provisioning operation completes (AGT-WSP-01: session per operation).
type Session struct {
	conn *grpc.ClientConn
	cl   openshellv1.OpenShellClient
	tok  string
}

// Dial opens a session to the gateway's OpenShell gRPC endpoint. endpoint is a
// host:port authority (any grpcs://, https:// or trailing-slash is normalized
// away). adminToken is a gateway-scoped token carrying the admin role;
// insecureSkipVerify skips TLS verification for the gateway's self-signed CA
// (OPENSHELL_GATEWAY_INSECURE equivalent).
func Dial(endpoint, adminToken string, insecureSkipVerify bool) (*Session, error) {
	authority := normalizeEndpoint(endpoint)
	if authority == "" {
		return nil, fmt.Errorf("empty gateway endpoint")
	}
	if adminToken == "" {
		return nil, fmt.Errorf("empty admin token for gateway %q", authority)
	}
	tlsCfg := &tls.Config{MinVersion: tls.VersionTLS12}
	if insecureSkipVerify {
		tlsCfg.InsecureSkipVerify = true
	}
	conn, err := grpc.NewClient(authority, grpc.WithTransportCredentials(credentials.NewTLS(tlsCfg)))
	if err != nil {
		return nil, fmt.Errorf("dial openshell gateway %q: %w", authority, err)
	}
	return &Session{conn: conn, cl: openshellv1.NewOpenShellClient(conn), tok: adminToken}, nil
}

// Close releases the underlying connection.
func (s *Session) Close() error { return s.conn.Close() }

func (s *Session) authCtx(ctx context.Context) context.Context {
	return metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+s.tok)
}

// EnsureWorkspace creates the named workspace when absent and is a no-op when it
// already exists (reconcile, do not create-or-skip).
func (s *Session) EnsureWorkspace(ctx context.Context, name string) error {
	ctx = s.authCtx(ctx)
	if _, err := s.cl.GetWorkspace(ctx, &openshellv1.GetWorkspaceRequest{Name: name}); err == nil {
		return nil
	} else if status.Code(err) != codes.NotFound {
		return fmt.Errorf("get workspace %q: %w", name, err)
	}
	if _, err := s.cl.CreateWorkspace(ctx, &openshellv1.CreateWorkspaceRequest{Name: name}); err != nil {
		if status.Code(err) == codes.AlreadyExists {
			return nil
		}
		return fmt.Errorf("create workspace %q: %w", name, err)
	}
	return nil
}

// EnsureMembers adds every (subject, role) in want to the workspace, skipping
// those already present with the desired role. It is ADDITIVE ONLY: it never
// removes a member it did not add, because a CITrust admits an open, lazily
// realized set of subjects and a workspace may hold unrelated members (agents, the
// admin provisioner). Stale membership is harmless on its own: actual access also
// requires the Keycloak role claim (PLAT-CITR-02), which is revoked by removing the
// subject from the trust's group. An AlreadyExists from the gateway is success.
func (s *Session) EnsureMembers(ctx context.Context, workspace string, want map[string]Role) error {
	ctx = s.authCtx(ctx)
	sel := &datamodelv1.WorkspaceSelector{Selection: &datamodelv1.WorkspaceSelector_Workspace{Workspace: workspace}}

	have := map[string]openshellv1.WorkspaceRole{}
	for pageToken := ""; ; {
		resp, err := s.cl.ListWorkspaceMembers(ctx, &openshellv1.ListWorkspaceMembersRequest{WorkspaceScope: sel, PageToken: pageToken})
		if err != nil {
			return fmt.Errorf("list members of workspace %q: %w", workspace, err)
		}
		for _, m := range resp.GetMembers() {
			have[m.GetPrincipalSubject()] = m.GetRole()
		}
		pageToken = resp.GetNextPageToken()
		if pageToken == "" {
			break
		}
	}

	var errs []error
	for subject, role := range want {
		if subject == "" {
			continue
		}
		if cur, ok := have[subject]; ok && cur == role.proto() {
			continue
		}
		_, err := s.cl.AddWorkspaceMember(ctx, &openshellv1.AddWorkspaceMemberRequest{
			WorkspaceScope:   sel,
			PrincipalSubject: subject,
			Role:             role.proto(),
		})
		if err != nil && status.Code(err) != codes.AlreadyExists {
			errs = append(errs, fmt.Errorf("add member %q to workspace %q: %w", subject, workspace, err))
		}
	}
	return errors.Join(errs...)
}

// normalizeEndpoint reduces a gateway endpoint to the host:port authority gRPC
// dials: it strips a grpcs://, grpc://, https:// or http:// scheme and any path,
// and defaults the port to 443 when none is present.
func normalizeEndpoint(endpoint string) string {
	e := strings.TrimSpace(endpoint)
	for _, scheme := range []string{"grpcs://", "grpc://", "https://", "http://"} {
		e = strings.TrimPrefix(e, scheme)
	}
	if i := strings.IndexByte(e, '/'); i >= 0 {
		e = e[:i]
	}
	if e == "" {
		return ""
	}
	if !strings.Contains(e, ":") {
		e += ":443"
	}
	return e
}
