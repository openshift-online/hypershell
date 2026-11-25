package managedClusters

import (
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/errors"
)

// Verifies: PLAT-CLUS-06
func TestNormalizeCapabilities(t *testing.T) {
	got, svcErr := NormalizeCapabilities([]string{"runtime/kata", "provider/aws", "network/vpn"})
	if svcErr != nil {
		t.Fatal(svcErr)
	}
	want := []string{"network/vpn", "provider/aws", "runtime/kata"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("capabilities = %v, want %v", got, want)
	}
	for _, labels := range [][]string{
		{},
		{"provider/AWS"},
		{"unknown/value"},
		{"provider/aws", "provider/ibm"},
		{"network/public", "network/public"},
	} {
		if _, svcErr := NormalizeCapabilities(labels); svcErr == nil {
			t.Errorf("NormalizeCapabilities(%v) unexpectedly succeeded", labels)
		}
	}
}

func TestNormalizeSelectorSortsAndDeduplicates(t *testing.T) {
	got, svcErr := NormalizeSelector([]string{"provider/aws", "network/public", "provider/aws"})
	if svcErr != nil {
		t.Fatal(svcErr)
	}
	want := []string{"network/public", "provider/aws"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("selector = %v, want %v", got, want)
	}
}

// The retired release labels are rejected in a capability set and in a selector,
// and the error names the purpose/ replacement.
// Verifies: PLAT-CLUS-06.05
func TestRetiredCapabilityLabelsAreRejected(t *testing.T) {
	for retired, replacement := range map[string]string{
		"runtime/release-check":  "purpose/release-check",
		"runtime/release-matrix": "purpose/release-matrix",
	} {
		for name, normalize := range map[string]func([]string) ([]string, *errors.ServiceError){
			"capabilities": NormalizeCapabilities,
			"selector":     NormalizeSelector,
		} {
			_, svcErr := normalize([]string{"provider/ibm", retired})
			if svcErr == nil {
				t.Errorf("%s accepted retired label %q", name, retired)
				continue
			}
			if svcErr.HttpCode != http.StatusBadRequest {
				t.Errorf("%s rejected %q with HTTP %d, want 400", name, retired, svcErr.HttpCode)
			}
			if !strings.Contains(svcErr.Reason, retired) || !strings.Contains(svcErr.Reason, replacement) {
				t.Errorf("%s error %q must name %q and its replacement %q", name, svcErr.Reason, retired, replacement)
			}
		}
	}
	// Other runtime/ values stay valid.
	if _, svcErr := NormalizeCapabilities([]string{"provider/ibm", "runtime/kata"}); svcErr != nil {
		t.Errorf("runtime/kata rejected: %v", svcErr)
	}
}

// Verifies: PLAT-CLUS-06.06
func TestReplaceRetiredCapabilityLabels(t *testing.T) {
	for _, tc := range []struct {
		name    string
		in      []string
		want    []string
		changed bool
	}{
		{"nothing retired keeps the input", []string{"provider/ibm", "network/public"}, []string{"provider/ibm", "network/public"}, false},
		{"replaces and sorts", []string{"provider/ibm", "runtime/release-matrix", "network/public"}, []string{"network/public", "provider/ibm", "purpose/release-matrix"}, true},
		{"drops a duplicate of the replacement", []string{"runtime/release-check", "purpose/release-check"}, []string{"purpose/release-check"}, true},
		{"keeps identity tokens", []string{"runtime/release-check", "managed-cluster/hub"}, []string{"managed-cluster/hub", "purpose/release-check"}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, changed := ReplaceRetiredCapabilityLabels(tc.in)
			if changed != tc.changed || !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("ReplaceRetiredCapabilityLabels(%v) = %v, %v; want %v, %v", tc.in, got, changed, tc.want, tc.changed)
			}
		})
	}
}
