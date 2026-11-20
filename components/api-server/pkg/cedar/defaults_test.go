package cedar

import "testing"

func TestParseDefaults(t *testing.T) {
	d, err := ParseDefaults("500m", "2", "1Gi", "4Gi")
	if err != nil {
		t.Fatalf("ParseDefaults: %v", err)
	}
	if d.CPURequestMillis != 500 {
		t.Fatalf("CPURequestMillis = %d, want 500", d.CPURequestMillis)
	}
	if d.CPULimitMillis != 2000 {
		t.Fatalf("CPULimitMillis = %d, want 2000", d.CPULimitMillis)
	}
	if d.MemoryRequestBytes != 1073741824 {
		t.Fatalf("MemoryRequestBytes = %d, want 1073741824", d.MemoryRequestBytes)
	}
	if d.MemoryLimitBytes != 4294967296 {
		t.Fatalf("MemoryLimitBytes = %d, want 4294967296", d.MemoryLimitBytes)
	}
	if d.GPUCount != 0 {
		t.Fatalf("GPUCount = %d, want 0", d.GPUCount)
	}
}

func TestParseDefaultsRejectsBadQuantity(t *testing.T) {
	if _, err := ParseDefaults("notacpu", "2", "1Gi", "4Gi"); err == nil {
		t.Fatal("expected error for invalid CPU request default")
	}
}
