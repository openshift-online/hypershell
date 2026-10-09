// Package directory maintains an in-memory projection of the Keycloak realm
// users and serves it to the API server (GAM-09). The API server never reads the
// Keycloak admin Secret; it reads this projection over the private in-cluster
// provisioner channel.
package directory

import (
	"context"
	"log"
	"strings"
	"sync"
	"time"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/provisioner/v1"
	"github.com/openshift-online/hypershell/components/control-plane/internal/keycloak"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	defaultRefreshInterval = 5 * time.Minute
	defaultSearchLimit     = 50
)

// startupRetryInterval is the cadence used while the snapshot is still empty at
// startup (e.g. the Keycloak realm import has not finished when the control
// plane comes up). It is a var so tests can shrink it.
var startupRetryInterval = 10 * time.Second

// lister is the slice of the Keycloak client the projection depends on.
type lister interface {
	ListRealmUsers(ctx context.Context) ([]keycloak.RealmUser, error)
}

// Projection is a periodically-refreshed snapshot of the realm directory.
type Projection struct {
	lister   lister
	interval time.Duration

	mu    sync.RWMutex
	users []keycloak.RealmUser
}

func NewProjection(l lister, interval time.Duration) *Projection {
	if interval <= 0 {
		interval = defaultRefreshInterval
	}
	return &Projection{lister: l, interval: interval}
}

// Run refreshes immediately, then on the configured interval until ctx is done.
// While the snapshot is still empty it retries on the shorter startupRetryInterval
// so the console "Add users" picker is not blank for a full refresh interval after
// a fresh deploy, when Keycloak may not have finished importing the realm yet.
func (p *Projection) Run(ctx context.Context) error {
	log.Printf("INFO keycloak directory projection started (interval=%s)", p.interval)
	p.refreshOnce(ctx)
	// ponytail: fast-poll while empty; a realm that is genuinely empty keeps
	// polling at startupRetryInterval forever rather than falling back to the
	// steady interval - fine, deployed realms always have users, and the first
	// one then shows up within startupRetryInterval instead of p.interval.
	for len(p.snapshot()) == 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(startupRetryInterval):
			p.refreshOnce(ctx)
		}
	}
	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			p.refreshOnce(ctx)
		}
	}
}

// refreshOnce replaces the snapshot. On error it keeps the previous snapshot so
// search stays available across transient Keycloak unavailability.
func (p *Projection) refreshOnce(ctx context.Context) {
	users, err := p.lister.ListRealmUsers(ctx)
	if err != nil {
		log.Printf("WARN keycloak directory refresh failed, keeping previous snapshot: %v", err)
		return
	}
	p.mu.Lock()
	p.users = users
	p.mu.Unlock()
	log.Printf("INFO keycloak directory refreshed (%d users)", len(users))
}

func (p *Projection) snapshot() []keycloak.RealmUser {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.users
}

// Search returns realm users whose username or name contains query (case-
// insensitive substring), capped at limit.
// ponytail: O(n) scan over the snapshot; fine for realm sizes served from memory.
func (p *Projection) Search(query string, limit int) []keycloak.RealmUser {
	if limit <= 0 || limit > defaultSearchLimit {
		limit = defaultSearchLimit
	}
	q := strings.ToLower(strings.TrimSpace(query))
	out := make([]keycloak.RealmUser, 0, limit)
	for _, u := range p.snapshot() {
		if q == "" || strings.Contains(strings.ToLower(u.Username), q) || strings.Contains(strings.ToLower(u.Name), q) {
			out = append(out, u)
			if len(out) >= limit {
				break
			}
		}
	}
	return out
}

// Resolve returns the realm user for an exact (case-insensitive) username.
func (p *Projection) Resolve(username string) (keycloak.RealmUser, bool) {
	want := strings.ToLower(strings.TrimSpace(username))
	if want == "" {
		return keycloak.RealmUser{}, false
	}
	for _, u := range p.snapshot() {
		if strings.ToLower(u.Username) == want {
			return u, true
		}
	}
	return keycloak.RealmUser{}, false
}

// Server adapts a Projection to the gRPC DirectoryService.
type Server struct {
	pb.UnimplementedDirectoryServiceServer
	projection *Projection
}

func NewServer(p *Projection) *Server { return &Server{projection: p} }

func (s *Server) SearchDirectory(_ context.Context, req *pb.SearchDirectoryRequest) (*pb.SearchDirectoryResponse, error) {
	users := s.projection.Search(req.GetQuery(), int(req.GetLimit()))
	resp := &pb.SearchDirectoryResponse{Users: make([]*pb.DirectoryUser, 0, len(users))}
	for _, u := range users {
		resp.Users = append(resp.Users, toProto(u))
	}
	return resp, nil
}

func (s *Server) ResolveUser(_ context.Context, req *pb.ResolveUserRequest) (*pb.ResolveUserResponse, error) {
	if req.GetUsername() == "" {
		return nil, status.Error(codes.InvalidArgument, "username is required")
	}
	u, found := s.projection.Resolve(req.GetUsername())
	if !found {
		return &pb.ResolveUserResponse{Found: false}, nil
	}
	return &pb.ResolveUserResponse{Found: true, User: toProto(u)}, nil
}

func toProto(u keycloak.RealmUser) *pb.DirectoryUser {
	return &pb.DirectoryUser{Username: u.Username, Name: u.Name, Email: u.Email, Subject: u.Subject}
}
