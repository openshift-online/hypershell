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
		want := gatewayNamespacePrefix + hex.EncodeToString(id.Bytes())
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
