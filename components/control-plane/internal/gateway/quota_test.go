package gateway

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func TestDesiredResourceQuotaSpec(t *testing.T) {
	tests := []struct {
		name     string
		quota    *QuotaConfig
		wantHard map[corev1.ResourceName]string
		wantSet  bool
	}{
		{
			name:    "nil quota is empty",
			quota:   nil,
			wantSet: false,
		},
		{
			name:    "all fields unset is empty",
			quota:   &QuotaConfig{},
			wantSet: false,
		},
		{
			// Spec § Configuration Examples, Example 1.
			name: "dev-small totals and pods",
			quota: &QuotaConfig{
				CPURequestTotal:    "1",
				CPULimitTotal:      "2",
				MemoryRequestTotal: "2Gi",
				MemoryLimitTotal:   "4Gi",
				PodCount:           10,
			},
			wantHard: map[corev1.ResourceName]string{
				corev1.ResourceRequestsCPU:    "1",
				corev1.ResourceLimitsCPU:      "2",
				corev1.ResourceRequestsMemory: "2Gi",
				corev1.ResourceLimitsMemory:   "4Gi",
				corev1.ResourcePods:           "10",
			},
			wantSet: true,
		},
		{
			// Spec § Configuration Examples, Example 3: pod count only.
			name:  "pod count only",
			quota: &QuotaConfig{PodCount: 20},
			wantHard: map[corev1.ResourceName]string{
				corev1.ResourcePods: "20",
			},
			wantSet: true,
		},
		{
			name: "ephemeral storage and pvc",
			quota: &QuotaConfig{
				EphemeralStorageTotal: "50Gi",
				PVCCount:              10,
			},
			wantHard: map[corev1.ResourceName]string{
				corev1.ResourceRequestsEphemeralStorage: "50Gi",
				corev1.ResourcePersistentVolumeClaims:   "10",
			},
			wantSet: true,
		},
		{
			name:    "zero counts are not set",
			quota:   &QuotaConfig{PodCount: 0, PVCCount: 0},
			wantSet: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hard, set, err := desiredResourceQuotaSpec(tt.quota)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if set != tt.wantSet {
				t.Fatalf("nonEmpty = %v, want %v", set, tt.wantSet)
			}
			if len(hard) != len(tt.wantHard) {
				t.Fatalf("hard has %d entries %v, want %d %v", len(hard), hard, len(tt.wantHard), tt.wantHard)
			}
			for name, want := range tt.wantHard {
				got, ok := hard[name]
				if !ok {
					t.Errorf("missing %s", name)
					continue
				}
				wantQ := resource.MustParse(want)
				if got.Cmp(wantQ) != 0 {
					t.Errorf("%s = %s, want %s", name, got.String(), want)
				}
			}
		})
	}
}

func TestDesiredResourceQuotaSpecInvalidQuantity(t *testing.T) {
	_, _, err := desiredResourceQuotaSpec(&QuotaConfig{CPURequestTotal: "tow"})
	if err == nil {
		t.Fatal("expected error for invalid quantity, got nil")
	}
}

func TestDesiredLimitRangeItem(t *testing.T) {
	tests := []struct {
		name        string
		quota       *QuotaConfig
		wantDefault map[corev1.ResourceName]string
		wantMax     map[corev1.ResourceName]string
		wantSet     bool
	}{
		{
			name:    "nil quota is empty",
			quota:   nil,
			wantSet: false,
		},
		{
			name:    "no container fields is empty",
			quota:   &QuotaConfig{CPURequestTotal: "1", PodCount: 5},
			wantSet: false,
		},
		{
			// Spec § Configuration Examples, Example 2.
			name: "defaults and maxima",
			quota: &QuotaConfig{
				ContainerCPURequestDefault:    "500m",
				ContainerCPULimitMax:          "4",
				ContainerMemoryRequestDefault: "512Mi",
				ContainerMemoryLimitMax:       "8Gi",
			},
			wantDefault: map[corev1.ResourceName]string{
				corev1.ResourceCPU:    "500m",
				corev1.ResourceMemory: "512Mi",
			},
			wantMax: map[corev1.ResourceName]string{
				corev1.ResourceCPU:    "4",
				corev1.ResourceMemory: "8Gi",
			},
			wantSet: true,
		},
		{
			// Spec § Configuration Examples, Example 1: defaults only, no max.
			name: "defaults only",
			quota: &QuotaConfig{
				ContainerCPURequestDefault:    "50m",
				ContainerMemoryRequestDefault: "64Mi",
			},
			wantDefault: map[corev1.ResourceName]string{
				corev1.ResourceCPU:    "50m",
				corev1.ResourceMemory: "64Mi",
			},
			wantSet: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			item, set, err := desiredLimitRangeItem(tt.quota)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if set != tt.wantSet {
				t.Fatalf("nonEmpty = %v, want %v", set, tt.wantSet)
			}
			if !tt.wantSet {
				return
			}
			if item.Type != corev1.LimitTypeContainer {
				t.Errorf("type = %s, want Container", item.Type)
			}
			assertResourceList(t, "defaultRequest", item.DefaultRequest, tt.wantDefault)
			assertResourceList(t, "max", item.Max, tt.wantMax)
		})
	}
}

func assertResourceList(t *testing.T, label string, got corev1.ResourceList, want map[corev1.ResourceName]string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s has %d entries %v, want %d %v", label, len(got), got, len(want), want)
	}
	for name, w := range want {
		g, ok := got[name]
		if !ok {
			t.Errorf("%s missing %s", label, name)
			continue
		}
		if g.Cmp(resource.MustParse(w)) != 0 {
			t.Errorf("%s[%s] = %s, want %s", label, name, g.String(), w)
		}
	}
}
