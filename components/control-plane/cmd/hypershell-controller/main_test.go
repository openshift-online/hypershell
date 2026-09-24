package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/openshift-online/hypershell/components/control-plane/internal/registration"
)

// scriptedRegistrar returns the scripted results in order, then the last one.
type scriptedRegistrar struct {
	results []error
	calls   int
}

func (s *scriptedRegistrar) Register(context.Context) (string, error) {
	i := s.calls
	if i >= len(s.results) {
		i = len(s.results) - 1
	}
	s.calls++
	if err := s.results[i]; err != nil {
		return "", err
	}
	return "2clusterid", nil
}

// Fail-Closed Startup: 403 and 409 are non-retryable and exit immediately with a
// clear message; any other failure is retried with backoff until it succeeds.
func TestRegisterWithBackoff(t *testing.T) {
	conflict := fmt.Errorf("%w: managed cluster name \"local-kind\" is held by record 2xyz", registration.ErrConflict)
	cases := []struct {
		name      string
		results   []error
		wantID    string
		wantCalls int
		wantErr   []string
	}{
		{name: "success", results: []error{nil}, wantID: "2clusterid", wantCalls: 1},
		{name: "transient then success", results: []error{errors.New("connection refused"), errors.New("503"), nil}, wantID: "2clusterid", wantCalls: 3},
		{name: "403 exits without retry", results: []error{registration.ErrForbidden}, wantCalls: 1, wantErr: []string{"managed-cluster-registrar"}},
		{name: "409 exits without retry quoting the API", results: []error{conflict}, wantCalls: 1, wantErr: []string{"held by record 2xyz", "operator"}},
		{name: "409 after transient failure", results: []error{errors.New("timeout"), conflict}, wantCalls: 2, wantErr: []string{"held by record 2xyz"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			reg := &scriptedRegistrar{results: tc.results}
			id, err := registerWithBackoff(context.Background(), reg, time.Millisecond)
			if reg.calls != tc.wantCalls {
				t.Fatalf("Register called %d times, want %d", reg.calls, tc.wantCalls)
			}
			if tc.wantErr == nil {
				if err != nil || id != tc.wantID {
					t.Fatalf("registerWithBackoff = (%q, %v), want (%q, nil)", id, err, tc.wantID)
				}
				return
			}
			if err == nil {
				t.Fatalf("registerWithBackoff = %q, want error", id)
			}
			for _, want := range tc.wantErr {
				if !strings.Contains(err.Error(), want) {
					t.Fatalf("error %q does not contain %q", err, want)
				}
			}
		})
	}
}

func TestRegisterWithBackoffHonoursCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	reg := &scriptedRegistrar{results: []error{errors.New("unreachable")}}
	if _, err := registerWithBackoff(ctx, reg, time.Hour); err == nil {
		t.Fatal("registerWithBackoff with a cancelled context: want error")
	}
}
