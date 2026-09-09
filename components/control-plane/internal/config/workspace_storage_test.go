package config

import (
	"strings"
	"testing"
)

func TestWorkspaceStorage(t *testing.T) {
	for _, test := range []struct {
		name, class, size, wantClass, wantSize, errorField string
	}{
		{name: "defaults"},
		{name: "class only", class: "sandbox-gp3", wantClass: "sandbox-gp3"},
		{name: "size only", size: "5Gi", wantSize: "5Gi"},
		{name: "trim and normalize", class: " fast.storage ", size: " 1024Mi ", wantClass: "fast.storage", wantSize: "1Gi"},
		{name: "uppercase class", class: "Storage", errorField: "GATEWAY_WORKSPACE_STORAGE_CLASS"},
		{name: "unsafe class", class: "storage\"\n[oidc]", errorField: "GATEWAY_WORKSPACE_STORAGE_CLASS"},
		{name: "malformed size", size: "large", errorField: "GATEWAY_WORKSPACE_DEFAULT_STORAGE_SIZE"},
		{name: "zero size", size: "0Gi", errorField: "GATEWAY_WORKSPACE_DEFAULT_STORAGE_SIZE"},
		{name: "negative size", size: "-1Gi", errorField: "GATEWAY_WORKSPACE_DEFAULT_STORAGE_SIZE"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("GATEWAY_WORKSPACE_STORAGE_CLASS", test.class)
			t.Setenv("GATEWAY_WORKSPACE_DEFAULT_STORAGE_SIZE", test.size)
			class, size, err := WorkspaceStorage()
			if test.errorField != "" {
				if err == nil || !strings.Contains(err.Error(), test.errorField) {
					t.Fatalf("expected error for %s, got %v", test.errorField, err)
				}
				if _, loadErr := Load(); loadErr == nil || !strings.Contains(loadErr.Error(), test.errorField) {
					t.Fatalf("startup must reject %s: %v", test.errorField, loadErr)
				}
				return
			}
			if err != nil || class != test.wantClass || size != test.wantSize {
				t.Fatalf("WorkspaceStorage() = %q, %q, %v; want %q, %q", class, size, err, test.wantClass, test.wantSize)
			}
		})
	}
}
