package rbac

import (
	"context"
	"errors"
	"log/slog"
	"sync"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/authz"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/environments"
	pkgserver "github.com/openshift-online/rh-trex-ai/components/api-server/pkg/server"
)

// Authorizer is HyperShell's single authorization policy (rbac-enforcement.spec.md).
// TRex calls it for every REST route a plugin declared with Resource and for
// every declared gRPC method, after authentication (specs/security/authorization
// in rh-trex-ai). It also implements authz.SubjectEnricher: Enrich provisions the
// local user and syncs JWT roles before Authorize runs.
type Authorizer struct {
	deps   Dependencies
	config AuthzConfig
	// enforcedFn decides enforcement per request; nil means config.EnforceRBAC.
	enforcedFn func() bool
	// fallback decides REST requests when no role-binding lookup is wired
	// (the roleBindings plugin is absent). It is the framework policy for
	// --enable-authz.
	fallback authz.Authorizer
	// grpcPassThrough allows every gRPC call. It is set only when the rbac
	// plugin is not loaded (see lazyAuthorizer).
	grpcPassThrough bool
}

var (
	_ authz.Authorizer      = &Authorizer{}
	_ authz.SubjectEnricher = &Authorizer{}
)

// Dependencies are the services the policy reads. Any of them may be nil when
// the plugin that provides it is not registered.
type Dependencies struct {
	Lookup           RoleBindingLookup
	Provisioner      UserProvisioner
	Syncer           JWTRoleSyncer
	ActivityRecorder DailyActivityRecorder
	// Clusters backs the watch-stream caller binding, which fails closed when
	// it is nil.
	Clusters RegisteredClusterResolver
	// ControlPlanes backs the control-plane identity. nil turns that identity
	// off (RBAC_SERVICE_ACCOUNTS alone applies).
	ControlPlanes RegisteredClusterResolver
	Gateways      GatewayClusterResolver
	Imports       ImportRepositoryResolver
}

// NewAuthorizer builds the policy from explicit dependencies.
func NewAuthorizer(deps Dependencies, config AuthzConfig) *Authorizer {
	return &Authorizer{deps: deps, config: config, fallback: authz.RequireIdentity()}
}

func (a *Authorizer) enforced() bool {
	if a.enforcedFn != nil {
		return a.enforcedFn()
	}
	return a.config.EnforceRBAC
}

// Enrich provisions the caller's local user and puts its id and JWT roles in
// the context. A provisioning failure is logged and the request continues
// without a user id, which role-binding authorization then denies.
func (a *Authorizer) Enrich(ctx context.Context, s authz.Subject) (context.Context, error) {
	if _, isGRPC := grpc.Method(ctx); isGRPC {
		ctx = withDeferredStatus(ctx)
		// No verified token (JWT off): there is no identity to provision.
		if s.Claims == nil || a.deps.Lookup == nil {
			return ctx, nil
		}
		return provisionUserForGRPC(ctx, a.deps.Provisioner, a.deps.Syncer), nil
	}
	if a.deps.Provisioner == nil {
		return ctx, nil
	}
	return provisionUserForHTTP(ctx, a.deps.Provisioner, a.deps.Syncer), nil
}

// Authorize decides one REST request or gRPC call.
func (a *Authorizer) Authorize(ctx context.Context, req authz.Request) (authz.Decision, error) {
	if req.GRPCMethod != "" {
		if a.grpcPassThrough {
			return authz.Allow(), nil
		}
		return a.authorizeGRPCRequest(ctx, req)
	}
	if req.HTTP == nil {
		return authz.Deny("request has no transport"), nil
	}
	if a.deps.Lookup == nil {
		return a.fallback.Authorize(ctx, req)
	}
	return a.authorizeHTTP(ctx, req.HTTP)
}

// authorizeGRPCRequest runs the watch-stream caller binding, then role-binding
// authorization, in that order. Every hypershell stream is a Watch, so the
// watch action marks a stream: it is decided when it opens (GRPCMsg nil), and
// the caller binding is checked again on the request message.
func (a *Authorizer) authorizeGRPCRequest(ctx context.Context, req authz.Request) (authz.Decision, error) {
	method := req.GRPCMethod
	var err error
	switch {
	case req.Action == authz.ActionWatch && req.GRPCMsg == nil:
		if isClusterBoundMethod(method) {
			_, _, err = callerRegisteredCluster(ctx, a.deps.Clusters)
		}
		if err == nil && a.deps.Lookup != nil {
			err = authorizeGRPC(ctx, method, nil, a.deps.Lookup, a.deps.ActivityRecorder, a.deps.ControlPlanes, a.deps.Gateways, a.deps.Imports, a.grpcConfig())
		}
	case req.Action == authz.ActionWatch:
		if isClusterBoundMethod(method) {
			clusterID, bound, lookupErr := callerRegisteredCluster(ctx, a.deps.Clusters)
			err = lookupErr
			if err == nil && bound {
				err = checkRequestCluster(req.GRPCMsg, clusterID)
			}
		}
	default:
		err = CheckClusterCallerBindingUnary(ctx, a.deps.Clusters, a.deps.Imports, method, req.GRPCMsg)
		if err == nil && a.deps.Lookup != nil {
			err = authorizeGRPC(ctx, method, req.GRPCMsg, a.deps.Lookup, a.deps.ActivityRecorder, a.deps.ControlPlanes, a.deps.Gateways, a.deps.Imports, a.grpcConfig())
		}
	}
	return decisionFromStatus(ctx, err)
}

func (a *Authorizer) grpcConfig() AuthzConfig {
	cfg := a.config
	cfg.EnforceRBAC = a.enforced()
	return cfg
}

// decisionFromStatus maps the gRPC status the checks return onto a decision.
// PermissionDenied and NotFound are denials, Unavailable is "could not decide".
// The checks also return InvalidArgument (a registered cluster that omits
// cluster_id, PLAT-CLUS-10) and Internal (an unexpected request type). A
// Decision cannot carry those codes, so they are held in the call context and
// returned by DeferredStatusUnaryInterceptor and DeferredStatusStreamInterceptor,
// which run right after authorization and before the handler.
func decisionFromStatus(ctx context.Context, err error) (authz.Decision, error) {
	if err == nil {
		return authz.Allow(), nil
	}
	st, ok := status.FromError(err)
	if !ok {
		return authz.Decision{}, err
	}
	switch st.Code() {
	case codes.PermissionDenied:
		return authz.Deny(st.Message()), nil
	case codes.NotFound:
		return authz.Decision{Reason: st.Message(), HideExistence: true}, nil
	case codes.Unavailable:
		return authz.Decision{}, err
	}
	holder := deferredStatusFrom(ctx)
	if holder == nil {
		// No holder means the deferred-status interceptors cannot report the
		// code; fail closed.
		return authz.Deny(st.Message()), nil
	}
	holder.set(err)
	return authz.Allow(), nil
}

// deferredStatus carries one non-authorization status from Authorize to the
// deferred-status interceptors.
type deferredStatus struct {
	mu  sync.Mutex
	err error
}

func (d *deferredStatus) set(err error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.err = err
}

// take returns the held status and clears it, so each stream message is
// checked on its own.
func (d *deferredStatus) take() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	err := d.err
	d.err = nil
	return err
}

type deferredStatusKey struct{}

func withDeferredStatus(ctx context.Context) context.Context {
	return context.WithValue(ctx, deferredStatusKey{}, &deferredStatus{})
}

func deferredStatusFrom(ctx context.Context) *deferredStatus {
	d, _ := ctx.Value(deferredStatusKey{}).(*deferredStatus)
	return d
}

// DeferredStatusUnaryInterceptor returns the status Authorize held for this
// call, if any. It must run after TRex's authorization interceptor.
func DeferredStatusUnaryInterceptor(ctx context.Context, req interface{}, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	if d := deferredStatusFrom(ctx); d != nil {
		if err := d.take(); err != nil {
			return nil, err
		}
	}
	return handler(ctx, req)
}

// DeferredStatusStreamInterceptor is the streaming counterpart: it checks the
// holder when the stream opens and after every received message.
func DeferredStatusStreamInterceptor(srv interface{}, ss grpc.ServerStream, _ *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
	d := deferredStatusFrom(ss.Context())
	if d == nil {
		return handler(srv, ss)
	}
	if err := d.take(); err != nil {
		return err
	}
	return handler(srv, &deferredStatusStream{ServerStream: ss, holder: d})
}

type deferredStatusStream struct {
	grpc.ServerStream
	holder *deferredStatus
}

func (s *deferredStatusStream) RecvMsg(m interface{}) error {
	if err := s.ServerStream.RecvMsg(m); err != nil {
		return err
	}
	return s.holder.take()
}

// DependencyProvider resolves the policy's dependencies from the environment's
// services. The rbac plugin registers it, because only plugins may import the
// other plugins' services.
type DependencyProvider func(env *environments.Env) Dependencies

var dependencyProvider DependencyProvider

// RegisterDependencyProvider sets the provider ForEnvironment uses.
func RegisterDependencyProvider(p DependencyProvider) {
	dependencyProvider = p
}

// ForEnvironment returns the authorizer every HyperShell environment uses.
// Dependencies are resolved on first use, after every service is registered.
// Enforcement is decided per request by Enforced.
func ForEnvironment(env *environments.Env) (authz.Authorizer, error) {
	if env == nil {
		return nil, errors.New("rbac: nil environment")
	}
	return &lazyAuthorizer{env: env}, nil
}

type lazyAuthorizer struct {
	once sync.Once
	env  *environments.Env
	a    *Authorizer
}

func (l *lazyAuthorizer) get() *Authorizer {
	l.once.Do(func() {
		enableAuthz := true
		if l.env.Config != nil && l.env.Config.Auth != nil {
			enableAuthz = l.env.Config.Auth.EnableAuthz
		}
		var deps Dependencies
		if dependencyProvider != nil {
			deps = dependencyProvider(l.env)
		}
		a := NewAuthorizer(deps, AuthzConfig{ServiceAccounts: ServiceAccountsFromEnv()})
		env := l.env
		a.enforcedFn = func() bool { return EnforcedFor(env) }
		a.fallback = authz.FromFlag(enableAuthz)
		if dependencyProvider == nil {
			// Only test binaries that do not load the rbac plugin get here.
			// They keep the framework policy on REST and no gRPC RBAC, as
			// before RBAC moved into the Authorizer.
			slog.Warn("rbac plugin not loaded: authorization uses the framework policy only")
			a.grpcPassThrough = true
		}
		l.a = a
	})
	return l.a
}

func (l *lazyAuthorizer) Authorize(ctx context.Context, req authz.Request) (authz.Decision, error) {
	return l.get().Authorize(ctx, req)
}

func (l *lazyAuthorizer) Enrich(ctx context.Context, s authz.Subject) (context.Context, error) {
	return l.get().Enrich(ctx, s)
}

func init() {
	// Report the statuses Authorize held (decisionFromStatus). They run right
	// after TRex's authorization interceptor.
	pkgserver.RegisterPostAuthGRPCUnaryInterceptor(DeferredStatusUnaryInterceptor)
	pkgserver.RegisterPostAuthGRPCStreamInterceptor(DeferredStatusStreamInterceptor)
}
