package gatewayAccess

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/openshift-online/hypershell/components/api-server/plugins/roleBindings"
	"github.com/openshift-online/hypershell/components/api-server/plugins/roles"
	"github.com/openshift-online/hypershell/components/api-server/plugins/users"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/db"
)

// Console access tiers. The management plane distinguishes three per-gateway
// roles; the console and gateway distinguish only the tiers below.
const (
	TierOwner = "owner"
	TierAdmin = "admin"
	TierUser  = "user"
)

const accessLockType db.LockType = "gateway_access"

func tierToRoleName(tier string) string {
	switch tier {
	case TierOwner:
		return roles.RoleGatewayOwner
	case TierAdmin:
		return roles.RoleGatewayAdmin
	case TierUser:
		return roles.RoleGatewayViewer
	default:
		return ""
	}
}

func roleNameToTier(name string) string {
	switch name {
	case roles.RoleGatewayOwner:
		return TierOwner
	case roles.RoleGatewayAdmin:
		return TierAdmin
	case roles.RoleGatewayViewer:
		return TierUser
	default:
		return ""
	}
}

var tierRank = map[string]int{TierUser: 1, TierAdmin: 2, TierOwner: 3}

// ListOptions follows the shared list contract. Role is a console tier filter
// (owner/admin/user); Search is a literal substring over username and name.
type ListOptions struct {
	Page   int
	Size   int
	Search string
	Role   string
	Sort   string
	Order  string
}

// GrantInput identifies the target of a grant by directory identity (GAM-04).
type GrantInput struct {
	Username string
	Subject  string
	Role     string
}

// GrantItem is the enriched access grant returned to callers (GAM-03 shape).
type GrantItem struct {
	RoleBindingID string
	UserID        string
	Username      string
	Name          *string
	Email         *string
	Role          string
	IsCreator     bool
	GrantedAt     time.Time
}

// Capabilities tells the console what the caller may do, so it can gate controls
// (GAM-UI-07/08). The server remains authoritative.
type Capabilities struct {
	CallerRole      string
	CanManageAccess bool
	CanManageOwners bool
}

// DirectoryCandidate is a realm user returned by the directory search (GAM-09).
type DirectoryCandidate struct {
	Username string
	Name     string
	Email    string
	Subject  string
}

type Service interface {
	List(ctx context.Context, gatewayID, callerUserID string, opts ListOptions) ([]GrantItem, int64, Capabilities, *APIError)
	Grant(ctx context.Context, gatewayID, callerUserID string, input GrantInput) (GrantItem, *APIError)
	ChangeRole(ctx context.Context, gatewayID, callerUserID, targetUserID, tier string) (GrantItem, *APIError)
	Revoke(ctx context.Context, gatewayID, callerUserID, targetUserID string) *APIError
	SearchDirectory(ctx context.Context, gatewayID, callerUserID, query string) ([]DirectoryCandidate, *APIError)
}

type service struct {
	dao       *dao
	bindings  roleBindings.RoleBindingService
	users     users.UserService
	directory DirectoryResolver
	locks     db.LockFactory
}

func NewService(d *dao, bindings roleBindings.RoleBindingService, userService users.UserService, directory DirectoryResolver, locks db.LockFactory) Service {
	if directory == nil {
		directory = unconfiguredResolver{}
	}
	return &service{dao: d, bindings: bindings, users: userService, directory: directory, locks: locks}
}

var _ Service = &service{}

// userTier returns the user's highest gateway tier on gatewayID (owner > admin
// > user), or "" if the user has no gateway-scoped binding there.
func (s *service) userTier(ctx context.Context, userID, gatewayID string) (string, *APIError) {
	summaries, err := s.bindings.FindBindingsByUserID(ctx, userID)
	if err != nil {
		return "", serverError("unable to resolve access")
	}
	best := ""
	for _, b := range summaries {
		if b.Scope != roleBindings.ScopeGateway || b.GatewayID == nil || *b.GatewayID != gatewayID {
			continue
		}
		t := roleNameToTier(b.RoleName)
		if t != "" && tierRank[t] > tierRank[best] {
			best = t
		}
	}
	return best, nil
}

func (s *service) List(ctx context.Context, gatewayID, callerUserID string, opts ListOptions) ([]GrantItem, int64, Capabilities, *APIError) {
	rows, total, err := s.dao.list(ctx, gatewayID, opts)
	if err != nil {
		return nil, 0, Capabilities{}, serverError("unable to list gateway access")
	}
	creatorID, err := s.dao.creatorUserID(ctx, gatewayID)
	if err != nil {
		return nil, 0, Capabilities{}, serverError("unable to resolve gateway creator")
	}
	items := make([]GrantItem, 0, len(rows))
	for _, r := range rows {
		items = append(items, grantItemFromRow(r, creatorID))
	}
	tier, aerr := s.userTier(ctx, callerUserID, gatewayID)
	if aerr != nil {
		return nil, 0, Capabilities{}, aerr
	}
	caps := Capabilities{
		CallerRole:      tier,
		CanManageAccess: tier == TierOwner || tier == TierAdmin,
		CanManageOwners: tier == TierOwner,
	}
	return items, total, caps, nil
}

func (s *service) Grant(ctx context.Context, gatewayID, callerUserID string, input GrantInput) (GrantItem, *APIError) {
	if tierToRoleName(input.Role) == "" {
		return GrantItem{}, validationProblem("role must be one of owner, admin, user")
	}
	if strings.TrimSpace(input.Username) == "" {
		return GrantItem{}, validationProblem("username is required")
	}
	if aerr := s.authorizeManage(ctx, callerUserID, gatewayID, input.Role, ""); aerr != nil {
		return GrantItem{}, aerr
	}

	// Resolve the identity against the Keycloak realm before any write (GAM-04).
	// Unknown -> 404, no pre-provision and no binding.
	dirUser, found, derr := s.directory.Resolve(ctx, input.Username)
	if derr != nil {
		if errors.Is(derr, ErrDirectoryUnavailable) {
			return GrantItem{}, serviceUnavailable("the identity directory is not available")
		}
		return GrantItem{}, serviceUnavailable("directory lookup failed")
	}
	if !found {
		return GrantItem{}, notFound(fmt.Sprintf("user %q was not found in the identity directory and cannot be granted access", input.Username))
	}

	// Pre-provision the HyperShell User from the directory record (upsert by
	// username), so the binding references a real user_id and the control plane
	// can resolve the Keycloak user by username.
	targetUserID, uerr := s.users.UpsertByUsername(ctx, strings.ToLower(dirUser.Username), optStr(dirUser.Email), optStr(dirUser.Name))
	if uerr != nil {
		return GrantItem{}, serverError("unable to provision user record")
	}

	release, aerr := s.lock(ctx, gatewayID)
	if aerr != nil {
		return GrantItem{}, aerr
	}
	defer release()

	// A grant that converges an existing owner to a lower tier is a change
	// (GAM-10); apply last-owner protection.
	if aerr := s.guardLastOwner(ctx, gatewayID, targetUserID, input.Role); aerr != nil {
		return GrantItem{}, aerr
	}
	if _, serr := s.bindings.SetGatewayRole(ctx, targetUserID, gatewayID, tierToRoleName(input.Role)); serr != nil {
		return GrantItem{}, fromServiceError(serr)
	}
	return s.grantItem(ctx, gatewayID, targetUserID)
}

func (s *service) ChangeRole(ctx context.Context, gatewayID, callerUserID, targetUserID, tier string) (GrantItem, *APIError) {
	if tierToRoleName(tier) == "" {
		return GrantItem{}, validationProblem("role must be one of owner, admin, user")
	}
	release, aerr := s.lock(ctx, gatewayID)
	if aerr != nil {
		return GrantItem{}, aerr
	}
	defer release()

	current, aerr := s.userTier(ctx, targetUserID, gatewayID)
	if aerr != nil {
		return GrantItem{}, aerr
	}
	if current == "" {
		return GrantItem{}, notFound("user has no access to this gateway")
	}
	if aerr := s.authorizeManage(ctx, callerUserID, gatewayID, tier, current); aerr != nil {
		return GrantItem{}, aerr
	}
	if aerr := s.guardLastOwner(ctx, gatewayID, targetUserID, tier); aerr != nil {
		return GrantItem{}, aerr
	}
	if _, serr := s.bindings.SetGatewayRole(ctx, targetUserID, gatewayID, tierToRoleName(tier)); serr != nil {
		return GrantItem{}, fromServiceError(serr)
	}
	return s.grantItem(ctx, gatewayID, targetUserID)
}

func (s *service) Revoke(ctx context.Context, gatewayID, callerUserID, targetUserID string) *APIError {
	release, aerr := s.lock(ctx, gatewayID)
	if aerr != nil {
		return aerr
	}
	defer release()

	current, aerr := s.userTier(ctx, targetUserID, gatewayID)
	if aerr != nil {
		return aerr
	}
	if current == "" {
		return notFound("user has no access to this gateway")
	}
	// Revoking to no access is an owner-tier operation when the target is an
	// owner (the "new tier" is none, i.e. not owner).
	if aerr := s.authorizeManage(ctx, callerUserID, gatewayID, "", current); aerr != nil {
		return aerr
	}
	if aerr := s.guardLastOwner(ctx, gatewayID, targetUserID, ""); aerr != nil {
		return aerr
	}
	if _, serr := s.bindings.RevokeGatewayAccess(ctx, targetUserID, gatewayID); serr != nil {
		return fromServiceError(serr)
	}
	return nil
}

func (s *service) SearchDirectory(ctx context.Context, gatewayID, callerUserID, query string) ([]DirectoryCandidate, *APIError) {
	tier, aerr := s.userTier(ctx, callerUserID, gatewayID)
	if aerr != nil {
		return nil, aerr
	}
	if tier != TierOwner && tier != TierAdmin {
		return nil, forbidden("only gateway owners and admins can search the directory")
	}
	const limit = 50
	candidates, err := s.directory.Search(ctx, query, limit)
	if err != nil {
		if errors.Is(err, ErrDirectoryUnavailable) {
			return nil, serviceUnavailable("the identity directory is not available")
		}
		return nil, serviceUnavailable("directory search failed")
	}
	out := make([]DirectoryCandidate, 0, len(candidates))
	for _, c := range candidates {
		out = append(out, DirectoryCandidate(c))
	}
	return out, nil
}

// authorizeManage enforces GAM-08: the caller must be owner or admin, and any
// operation touching the Owner tier (assigning owner, or changing/revoking a
// user who currently holds owner) requires the caller to be an owner.
// newTier is "" for revoke; currentTier is "" for a fresh grant.
func (s *service) authorizeManage(ctx context.Context, callerUserID, gatewayID, newTier, currentTier string) *APIError {
	caller, aerr := s.userTier(ctx, callerUserID, gatewayID)
	if aerr != nil {
		return aerr
	}
	if caller != TierOwner && caller != TierAdmin {
		return forbidden("only gateway owners and admins can manage access")
	}
	if (newTier == TierOwner || currentTier == TierOwner) && caller != TierOwner {
		return forbidden("only gateway owners can assign, change, or revoke the owner role")
	}
	return nil
}

// guardLastOwner rejects an operation (demote or revoke) that would remove the
// last remaining owner (GAM-07). newTier is "" for revoke.
func (s *service) guardLastOwner(ctx context.Context, gatewayID, targetUserID, newTier string) *APIError {
	owners, err := s.dao.ownerUserIDs(ctx, gatewayID)
	if err != nil {
		return serverError("unable to evaluate gateway owners")
	}
	isOwner := false
	for _, id := range owners {
		if id == targetUserID {
			isOwner = true
			break
		}
	}
	if !isOwner || newTier == TierOwner {
		return nil
	}
	if len(owners) <= 1 {
		return conflictErr("this gateway must keep at least one owner; assign another owner before demoting or removing this one")
	}
	return nil
}

func (s *service) grantItem(ctx context.Context, gatewayID, userID string) (GrantItem, *APIError) {
	row, found, err := s.dao.getForUser(ctx, gatewayID, userID)
	if err != nil {
		return GrantItem{}, serverError("unable to load grant")
	}
	if !found {
		return GrantItem{}, serverError("grant not found after write")
	}
	creatorID, err := s.dao.creatorUserID(ctx, gatewayID)
	if err != nil {
		return GrantItem{}, serverError("unable to resolve gateway creator")
	}
	return grantItemFromRow(row, creatorID), nil
}

func grantItemFromRow(r accessRow, creatorID string) GrantItem {
	return GrantItem{
		RoleBindingID: r.RoleBindingID,
		UserID:        r.UserID,
		Username:      r.Username,
		Name:          r.Name,
		Email:         r.Email,
		Role:          roleNameToTier(r.RoleName),
		IsCreator:     r.UserID == creatorID && r.RoleName == roles.RoleGatewayOwner,
		GrantedAt:     r.GrantedAt,
	}
}

func (s *service) lock(ctx context.Context, gatewayID string) (func(), *APIError) {
	if s.locks == nil {
		return func() {}, nil
	}
	owner, err := s.locks.NewAdvisoryLock(ctx, "gateway-access-"+gatewayID, accessLockType)
	if err != nil {
		return nil, serverError("unable to acquire access lock")
	}
	return func() { s.locks.Unlock(ctx, owner) }, nil
}

func optStr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
