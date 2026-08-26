package main

import (
	"testing"

	"k8s.io/apimachinery/pkg/runtime"
	dynamicclient "k8s.io/client-go/dynamic"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	kubernetes "k8s.io/client-go/kubernetes"
)

func TestManagedDatabaseWatchEligible(t *testing.T) {
	typed := &kubernetes.Clientset{}
	var dynamic dynamicclient.Interface = dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
	for _, tt := range []struct {
		name    string
		typed   bool
		dynamic bool
		want    bool
	}{
		{"both clients", true, true, true},
		{"no typed client", false, true, false},
		{"no dynamic client", true, false, false},
		{"no clients", false, false, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			gotTyped := typed
			if !tt.typed {
				gotTyped = nil
			}
			var gotDynamic = dynamic
			if !tt.dynamic {
				gotDynamic = nil
			}
			if got := managedDatabaseWatchEligible(gotTyped, gotDynamic); got != tt.want {
				t.Fatalf("eligible = %v, want %v", got, tt.want)
			}
		})
	}
}
