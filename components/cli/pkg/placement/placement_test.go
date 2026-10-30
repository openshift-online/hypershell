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
	intent, err := BuildIntent(nil, nil, false)
	if err != nil {
		t.Fatalf("BuildIntent(nil, nil, false) error = %v", err)
	}
	if intent != nil {
		t.Fatalf("BuildIntent(nil, nil, false) = %v, want nil (pinned shape)", intent)
	}
}

func TestBuildIntentSelectorShape(t *testing.T) {
	intent, err := BuildIntent([]string{"network/public", "provider/ibm"}, nil, false)
	if err != nil {
		t.Fatalf("BuildIntent error = %v", err)
	}
	want := map[string]interface{}{"selector": []string{"network/public", "provider/ibm"}}
	if !reflect.DeepEqual(intent, want) {
		t.Fatalf("intent = %v, want %v", intent, want)
	}
}

func TestBuildIntentExcludedSelectorShape(t *testing.T) {
	intent, err := BuildIntent([]string{"provider/ibm"}, []string{"runtime/kata"}, false)
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

func TestBuildIntentLocalKindShape(t *testing.T) {
	intent, err := BuildIntent(nil, nil, true)
	if err != nil {
		t.Fatalf("BuildIntent error = %v", err)
	}
	want := map[string]interface{}{"mode": "local-kind"}
	if !reflect.DeepEqual(intent, want) {
		t.Fatalf("intent = %v, want %v", intent, want)
	}
}

func TestBuildIntentRejectsLocalKindWithSelectors(t *testing.T) {
	for _, tc := range []struct {
		selectors, excludes []string
	}{
		{[]string{"network/public"}, nil},
		{nil, []string{"runtime/kata"}},
	} {
		_, err := BuildIntent(tc.selectors, tc.excludes, true)
		if err == nil {
			t.Fatalf("BuildIntent(%v, %v, true) = nil error, want local-kind exclusivity error", tc.selectors, tc.excludes)
		}
		if !strings.Contains(err.Error(), "--local-kind") {
			t.Fatalf("error %q should mention --local-kind", err.Error())
		}
	}
}

func TestBuildIntentRejectsExclusionWithoutSelector(t *testing.T) {
	_, err := BuildIntent(nil, []string{"runtime/kata"}, false)
	if err == nil {
		t.Fatal("BuildIntent(nil, [runtime/kata], false) = nil error, want exclusion-requires-selector error")
	}
	if !strings.Contains(err.Error(), "--placement-exclude-selector requires") {
		t.Fatalf("error %q should name the required flag", err.Error())
	}
}

func TestBuildIntentRejectsInvalidLabelsBeforeBuilding(t *testing.T) {
	if _, err := BuildIntent([]string{"network/public,provider/ibm"}, nil, false); err == nil {
		t.Fatal("comma-separated selector value should be rejected")
	}
	if _, err := BuildIntent([]string{"provider/ibm"}, []string{"cloud/aws"}, false); err == nil {
		t.Fatal("invalid excluded label should be rejected")
	}
}

func TestBuildIntentPassesDuplicatesThrough(t *testing.T) {
	intent, err := BuildIntent([]string{"network/public", "network/public"}, nil, false)
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
