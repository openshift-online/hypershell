// Package rbac wires HyperShell's authorization policy (pkg/rbac) to the
// plugins' services. Every environment returns rbac.ForEnvironment from its
// OverrideAuthorizer hook; TRex then applies the policy to every declared REST
// route and gRPC method.
package rbac

import (
	pkgrbac "github.com/openshift-online/hypershell/components/api-server/pkg/rbac"
)

func init() {
	pkgrbac.RegisterDependencyProvider(dependencies)
}
