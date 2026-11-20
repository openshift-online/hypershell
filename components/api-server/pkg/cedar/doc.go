// Package cedar is the shared HyperShell policy-language package for
// PlatformGatewayPolicy (specs/platform/platform-gateway-policy.spec.md).
//
// It holds the single, versioned, driver-neutral Cedar schema (PLAT-PGP-03),
// the Kubernetes quantity normalization that maps request quantities to the
// integer units the schema names (PLAT-PGP-03), the write-time validation of
// forbid-only policy text against that schema including the default-request
// check (PLAT-PGP-04), the @id attribution the control plane stamps onto every
// statement in a bundle (PLAT-PGP-14), and the best-effort display renderer
// that turns recognized statement shapes into plain words (PLAT-PGP-17).
//
// The package is shared so the three Go components agree on one schema and one
// normalization (PLAT-PGP-03.02): the api-server validates with it, the control
// plane builds bundles with it, and the policy interceptor evaluates with it.
// It lives in the api-server module because the control-plane module already
// depends on the api-server module through a replace directive, so both can
// import this one package.
//
// # cedar-go capability decision (PLAT-PGP-05)
//
// PLAT-PGP-05 requires confirming that the pinned cedar-go release validates
// policies against a schema (rejecting unknown attributes and type errors) with
// usable diagnostics before committing to the cedar-go validation path. This
// package pins github.com/cedar-policy/cedar-go and uses its
// x/exp/schema/validate package: Validator.Policy reports unknown context
// attributes ("attribute `cpuRequest` in context ... not found") and type
// mismatches ("unexpected type: expected ... Long but saw String") as Go
// errors. The capability is sufficient, so HyperShell validates with cedar-go
// (ValidatePolicy) and does not need the Rust Cedar validator fallback. The
// interceptor evaluates accepted policies with the same library.
package cedar
