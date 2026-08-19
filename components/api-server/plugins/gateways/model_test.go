package gateways

import (
	"encoding/hex"
	"regexp"
	"testing"

	"github.com/segmentio/ksuid"
)

func TestBeforeCreateAssignsUniqueKubernetesNamespaces(t *testing.T) {
	first := &Gateway{Namespace: "caller-selected"}
	second := &Gateway{}

	if err := first.BeforeCreate(nil); err != nil {
		t.Fatalf("assign first gateway identity: %v", err)
	}
	if err := second.BeforeCreate(nil); err != nil {
		t.Fatalf("assign second gateway identity: %v", err)
	}

	dnsLabel := regexp.MustCompile(`^[a-z0-9](?:[-a-z0-9]*[a-z0-9])?$`)
	for _, gateway := range []*Gateway{first, second} {
		id, err := ksuid.Parse(gateway.ID)
		if err != nil {
			t.Fatalf("parse generated gateway ID: %v", err)
		}
		want := gatewayNamespacePrefix + hex.EncodeToString(id.Payload()[:8])
		if gateway.Namespace != want {
			t.Errorf("namespace = %q, want %q", gateway.Namespace, want)
		}
		if len(gateway.Namespace) > 63 || !dnsLabel.MatchString(gateway.Namespace) {
			t.Errorf("namespace %q is not a Kubernetes DNS label", gateway.Namespace)
		}
	}

	if first.Namespace == second.Namespace {
		t.Errorf("two gateways received the same namespace %q", first.Namespace)
	}
}

func TestBeforeCreateInitializesGenerationUnconverged(t *testing.T) {
	gw := &Gateway{}
	if err := gw.BeforeCreate(nil); err != nil {
		t.Fatalf("assign gateway identity: %v", err)
	}
	if gw.Generation != 1 {
		t.Errorf("generation = %d, want 1", gw.Generation)
	}
	if gw.ObservedGeneration != 0 {
		t.Errorf("observed_generation = %d, want 0", gw.ObservedGeneration)
	}
	if gw.ObservedGeneration >= gw.Generation {
		t.Errorf("new gateway must be unconverged: observed %d >= generation %d",
			gw.ObservedGeneration, gw.Generation)
	}
}

func TestDesiredStateChanged(t *testing.T) {
	str := func(s string) *string { return &s }
	base := func() *Gateway {
		return &Gateway{
			Name: "gw", FleetId: "f1", ClusterId: "c1", ReleaseId: "r1", DatabaseId: "d1",
			Image: str("img:1"), Oidc: str(`{"issuer":"a"}`),
		}
	}

	tests := []struct {
		name   string
		mutate func(*Gateway)
		want   bool
	}{
		{"no change", func(*Gateway) {}, false},
		{"identity name ignored", func(g *Gateway) { g.Name = "renamed" }, false},
		{"identity fleet ignored", func(g *Gateway) { g.FleetId = "f2" }, false},
		{"observed phase ignored", func(g *Gateway) { g.Phase = str("Running") }, false},
		{"observed route_address ignored", func(g *Gateway) { g.RouteAddress = str("grpcs://x") }, false},
		{"observed generation ignored", func(g *Gateway) { g.ObservedGeneration = 5 }, false},
		{"desired image changed", func(g *Gateway) { g.Image = str("img:2") }, true},
		{"desired cluster changed", func(g *Gateway) { g.ClusterId = "c2" }, true},
		{"desired oidc changed", func(g *Gateway) { g.Oidc = str(`{"issuer":"b"}`) }, true},
		{"desired route set from nil", func(g *Gateway) { g.Route = str(`{"host":"h"}`) }, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			current := base()
			next := base()
			tc.mutate(next)
			if got := desiredStateChanged(current, next); got != tc.want {
				t.Errorf("desiredStateChanged = %v, want %v", got, tc.want)
			}
		})
	}
}
