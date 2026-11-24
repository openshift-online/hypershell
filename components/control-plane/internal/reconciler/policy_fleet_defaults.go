package reconciler

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"google.golang.org/grpc"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"github.com/openshift-online/hypershell/components/api-server/pkg/cedar"
)

// ErrNoFleetDefaults reports that the api-server has not provided the fleet
// defaults yet. The controller then writes no bundle and renders no sidecar,
// so the gateway's protected RPCs fail closed (PLAT-PGP-12) instead of being
// evaluated against made-up defaults.
var ErrNoFleetDefaults = errors.New("platform policy settings carry no fleet defaults yet")

// FleetDefaults are the sandbox resource defaults the api-server accepted
// (PLAT-PGP-08): the quantities as written, passed to the interceptor sidecar,
// and their normalized schema values, used to validate bundles.
type FleetDefaults struct {
	CPURequest    string
	CPULimit      string
	MemoryRequest string
	MemoryLimit   string
	Normalized    cedar.Defaults
}

// FleetDefaultsFromSettings reads the accepted fleet defaults from the
// PlatformPolicySettings the api-server streams to the control plane. The
// controller has no defaults configuration of its own.
func FleetDefaultsFromSettings(s *pb.PlatformPolicySettings) (FleetDefaults, error) {
	d := s.GetDefaults()
	if d == nil || d.GetCpuRequest() == "" {
		return FleetDefaults{}, ErrNoFleetDefaults
	}
	out := FleetDefaults{
		CPURequest:    d.GetCpuRequest(),
		CPULimit:      d.GetCpuLimit(),
		MemoryRequest: d.GetMemoryRequest(),
		MemoryLimit:   d.GetMemoryLimit(),
	}
	n, err := cedar.ParseDefaults(out.CPURequest, out.CPULimit, out.MemoryRequest, out.MemoryLimit)
	if err != nil {
		return FleetDefaults{}, fmt.Errorf("fleet defaults from platform policy settings: %w", err)
	}
	out.Normalized = n
	return out, nil
}

// FleetDefaultsSource holds the last fleet defaults received through
// PlatformPolicySettings. The bundle reconciler updates it on every pass; the
// sidecar provider reads it, and asks the api-server directly when nothing was
// received yet. Safe for concurrent use.
type FleetDefaultsSource struct {
	settings pb.PlatformPolicySettingsServiceClient

	mu      sync.RWMutex
	current *FleetDefaults
}

// NewFleetDefaultsSource builds a source backed by the api-server connection.
func NewFleetDefaultsSource(conn grpc.ClientConnInterface) *FleetDefaultsSource {
	return &FleetDefaultsSource{settings: pb.NewPlatformPolicySettingsServiceClient(conn)}
}

// Update records the defaults carried by the settings and returns them.
func (s *FleetDefaultsSource) Update(settings *pb.PlatformPolicySettings) (FleetDefaults, error) {
	d, err := FleetDefaultsFromSettings(settings)
	if err != nil {
		return FleetDefaults{}, err
	}
	s.mu.Lock()
	s.current = &d
	s.mu.Unlock()
	return d, nil
}

// Get returns the received defaults, reading the settings from the api-server
// when none were received yet. It never returns a built-in value.
func (s *FleetDefaultsSource) Get(ctx context.Context) (FleetDefaults, error) {
	s.mu.RLock()
	cur := s.current
	s.mu.RUnlock()
	if cur != nil {
		return *cur, nil
	}
	resp, err := s.settings.GetPlatformPolicySettings(ctx, &pb.GetPlatformPolicySettingsRequest{})
	if err != nil {
		return FleetDefaults{}, fmt.Errorf("get platform policy settings: %w", err)
	}
	return s.Update(resp.GetSettings())
}
