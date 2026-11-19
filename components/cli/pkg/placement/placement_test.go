package placement

import (
	"reflect"
	"strings"
	"testing"
)

func TestValidateLabelAcceptsEveryNamespace(t *testing.T) {
	for _, label := range []string{
		"provider/aws", "provider/ibm", "provider/kind",
		"network/public", "network/vpn",
		"region/us-east-1", "region/eu-central-1",
		"runtime/kata", "runtime/default",
		"environment/kind",
	} {
		if err := ValidateLabel(label); err != nil {
			t.Errorf("ValidateLabel(%q) = %v, want nil", label, err)
		}
	}
}

// TestValidateLabelAcceptsManagedClusterIdentityNamespace proves the CLI recognizes
// the admin identity namespace client-side so a managed-cluster/<name> token is not
// rejected fail-fast as an unknown namespace; the server stays authoritative
// (CLI-PLC-05).
func TestValidateLabelAcceptsManagedClusterIdentityNamespace(t *testing.T) {
	for _, label := range []string{"managed-cluster/hyp0-mc1", "managed-cluster/eks-1"} {
		if err := ValidateLabel(label); err != nil {
			t.Errorf("ValidateLabel(%q) = %v, want nil", label, err)
		}
	}
	// A malformed value under the identity namespace is still rejected by the
	// DNS-label check, so the token shape is validated client-side.
	if err := ValidateLabel("managed-cluster/UpperCase"); err == nil {
		t.Error("ValidateLabel(managed-cluster/UpperCase) = nil, want a DNS-label error")
	}
}

// TestBuildIntentCarriesManagedClusterIdentitySelector proves the identity token rides
// the existing --placement-selector and --placement-exclude-selector flags into the
// request body, with no cluster_id (CLI-PLC-05.01, CLI-PLC-05.02).
func TestBuildIntentCarriesManagedClusterIdentitySelector(t *testing.T) {
	intent, err := BuildIntent([]string{"managed-cluster/hyp0-mc1"}, nil)
	if err != nil {
		t.Fatalf("BuildIntent error = %v", err)
	}
	want := map[string]interface{}{"selector": []string{"managed-cluster/hyp0-mc1"}}
	if !reflect.DeepEqual(intent, want) {
		t.Fatalf("intent = %v, want %v", intent, want)
	}
	if _, hasCluster := intent["cluster_id"]; hasCluster {
		t.Fatalf("intent = %v, must not carry cluster_id", intent)
	}

	excluded, err := BuildIntent([]string{"provider/ibm"}, []string{"managed-cluster/hyp0-mc1"})
	if err != nil {
		t.Fatalf("BuildIntent error = %v", err)
	}
	wantExcluded := map[string]interface{}{
		"selector":         []string{"provider/ibm"},
		"excludedSelector": []string{"managed-cluster/hyp0-mc1"},
	}
	if !reflect.DeepEqual(excluded, wantExcluded) {
		t.Fatalf("intent = %v, want %v", excluded, wantExcluded)
	}
}

func TestValidateLabelRejectsMalformedValues(t *testing.T) {
	cases := []struct {
		label string
		want  string
	}{
		{"", "must not be empty"},
		{"network/public,provider/ibm", "must not contain a comma"},
		{"network/p ublic", "must not contain whitespace"},
		{"network", "must be namespace/value"},
		{"network/public/extra", "must be namespace/value"},
		{"cloud/aws", "unknown namespace"},
		{"Network/public", "unknown namespace"},
		{"network/UpperCase", "lowercase DNS label"},
		{"network/-leading", "lowercase DNS label"},
		{"network/trailing-", "lowercase DNS label"},
	}
	for _, tc := range cases {
		err := ValidateLabel(tc.label)
		if err == nil {
			t.Errorf("ValidateLabel(%q) = nil, want error containing %q", tc.label, tc.want)
			continue
		}
		if !strings.Contains(err.Error(), tc.want) {
			t.Errorf("ValidateLabel(%q) = %q, want error containing %q", tc.label, err.Error(), tc.want)
		}
	}
}

func TestBuildIntentReturnsNilWhenNoPlacementFlags(t *testing.T) {
	intent, err := BuildIntent(nil, nil)
	if err != nil {
		t.Fatalf("BuildIntent(nil, nil) error = %v", err)
	}
	if intent != nil {
		t.Fatalf("BuildIntent(nil, nil) = %v, want nil (pinned shape)", intent)
	}
}

func TestBuildIntentSelectorShape(t *testing.T) {
	intent, err := BuildIntent([]string{"network/public", "provider/ibm"}, nil)
	if err != nil {
		t.Fatalf("BuildIntent error = %v", err)
	}
	want := map[string]interface{}{"selector": []string{"network/public", "provider/ibm"}}
	if !reflect.DeepEqual(intent, want) {
		t.Fatalf("intent = %v, want %v", intent, want)
	}
}

func TestBuildIntentExcludedSelectorShape(t *testing.T) {
	intent, err := BuildIntent([]string{"provider/ibm"}, []string{"runtime/kata"})
	if err != nil {
		t.Fatalf("BuildIntent error = %v", err)
	}
	want := map[string]interface{}{
		"selector":         []string{"provider/ibm"},
		"excludedSelector": []string{"runtime/kata"},
	}
	if !reflect.DeepEqual(intent, want) {
		t.Fatalf("intent = %v, want %v", intent, want)
	}
}

func TestBuildIntentKindSelectorShape(t *testing.T) {
	// CLI-PLC-01.10: Kind local development uses the ordinary capability
	// selector, not a dedicated mode.
	intent, err := BuildIntent([]string{"provider/kind"}, nil)
	if err != nil {
		t.Fatalf("BuildIntent error = %v", err)
	}
	want := map[string]interface{}{"selector": []string{"provider/kind"}}
	if !reflect.DeepEqual(intent, want) {
		t.Fatalf("intent = %v, want %v", intent, want)
	}
}

func TestBuildIntentRejectsExclusionWithoutSelector(t *testing.T) {
	_, err := BuildIntent(nil, []string{"runtime/kata"})
	if err == nil {
		t.Fatal("BuildIntent(nil, [runtime/kata]) = nil error, want exclusion-requires-selector error")
	}
	if !strings.Contains(err.Error(), "--placement-exclude-selector requires") {
		t.Fatalf("error %q should name the required flag", err.Error())
	}
}

func TestBuildIntentRejectsInvalidLabelsBeforeBuilding(t *testing.T) {
	if _, err := BuildIntent([]string{"network/public,provider/ibm"}, nil); err == nil {
		t.Fatal("comma-separated selector value should be rejected")
	}
	if _, err := BuildIntent([]string{"provider/ibm"}, []string{"cloud/aws"}); err == nil {
		t.Fatal("invalid excluded label should be rejected")
	}
}

func TestBuildIntentPassesDuplicatesThrough(t *testing.T) {
	intent, err := BuildIntent([]string{"network/public", "network/public"}, nil)
	if err != nil {
		t.Fatalf("BuildIntent error = %v", err)
	}
	want := map[string]interface{}{"selector": []string{"network/public", "network/public"}}
	if !reflect.DeepEqual(intent, want) {
		t.Fatalf("intent = %v, want %v (the server collapses duplicates)", intent, want)
	}
}

func TestAvailabilityRequestCarriesSingleSelectorEntry(t *testing.T) {
	body := AvailabilityRequest([]string{"network/public", "provider/ibm"})
	want := map[string]interface{}{"selectors": [][]string{{"network/public", "provider/ibm"}}}
	if !reflect.DeepEqual(body, want) {
		t.Fatalf("body = %v, want %v", body, want)
	}
}
