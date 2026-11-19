package openshellclient

import (
	"context"
	"fmt"

	datamodelv1 "github.com/openshift-online/hypershell/components/control-plane/internal/openshellclient/gen/datamodelv1"
	openshellv1 "github.com/openshift-online/hypershell/components/control-plane/internal/openshellclient/gen/openshellv1"
)

// ServiceDiscovery is OpenShell's application-ingress API
// (openshell.v1.ServiceDiscovery). A snapshot or watch reports the service routes
// supervisors have installed in exactly one workspace: hostname, owning
// supervisor endpoint, installation version, declaration revision and expiry.
// The routes are installation evidence, not application health or ingress
// readiness, and discovery requires a token carrying the sandbox:read scope with
// the workspace user role.
//
// The watch is bounded (the gateway ends it after a short lifetime), so a clean
// end of stream means "relist", not failure, and an error is never an empty
// replacement: callers keep the last known state and expire it themselves.

// workspaceSelector builds the single-workspace selector discovery requires.
// Discovery rejects an omitted or all-workspaces selector on the pinned build.
func workspaceSelector(workspace string) *datamodelv1.WorkspaceSelector {
	return &datamodelv1.WorkspaceSelector{
		Selection: &datamodelv1.WorkspaceSelector_Workspace{Workspace: workspace},
	}
}

// GetServiceRouteSnapshot reads one authoritative route snapshot for the named
// workspace. An empty workspace is a caller error: discovery takes exactly one
// explicit workspace selector, and an omitted selector is rejected upstream.
func (s *Session) GetServiceRouteSnapshot(ctx context.Context, workspace string) (*openshellv1.ServiceRouteSnapshot, error) {
	if workspace == "" {
		return nil, fmt.Errorf("get service route snapshot: empty workspace selector")
	}
	if s.disc == nil {
		return nil, fmt.Errorf("get service route snapshot: session has no discovery client")
	}
	resp, err := s.disc.GetServiceRouteSnapshot(s.authCtx(ctx), &openshellv1.GetServiceRouteSnapshotRequest{
		WorkspaceScope: workspaceSelector(workspace),
	})
	if err != nil {
		return nil, fmt.Errorf("get service route snapshot for workspace %q: %w", workspace, err)
	}
	return resp.GetSnapshot(), nil
}

// WatchServiceRoutes opens the bounded route watch for the named workspace and
// returns the stream. The first message is an initial full replacement view and
// later messages are replacements, including lease-only renewals. The gateway
// ends the stream by design; io.EOF from Recv is the normal end of a watch
// lifetime, while any other error is a discovery failure that must not be
// treated as an empty replacement. The caller owns Close.
func (s *Session) WatchServiceRoutes(ctx context.Context, workspace string) (openshellv1.ServiceDiscovery_WatchServiceRoutesClient, error) {
	if workspace == "" {
		return nil, fmt.Errorf("watch service routes: empty workspace selector")
	}
	if s.disc == nil {
		return nil, fmt.Errorf("watch service routes: session has no discovery client")
	}
	stream, err := s.disc.WatchServiceRoutes(s.authCtx(ctx), &openshellv1.WatchServiceRoutesRequest{
		WorkspaceScope: workspaceSelector(workspace),
	})
	if err != nil {
		return nil, fmt.Errorf("watch service routes for workspace %q: %w", workspace, err)
	}
	return stream, nil
}
