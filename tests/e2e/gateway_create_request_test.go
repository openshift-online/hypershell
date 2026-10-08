package e2e

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestGatewayCreateRequestForDriver(t *testing.T) {
	const (
		name = "e2e-gateway"
		oidc = `{"issuer":"https://issuer.example","audience":"hypershell-frontend"}`
	)

	tests := []struct {
		name       string
		driverName string
		placement  map[string]any
	}{
		{
			name:       "Kind uses its local placement",
			driverName: "kind",
			placement:  map[string]any{"mode": "local-kind"},
		},
		{
			name:       "OpenShift uses managed AWS public placement",
			driverName: "openshift",
			placement:  map[string]any{"network": "public", "provider": "aws"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload := gatewayCreateRequestForDriver(name, tt.driverName, oidc)
			encoded, err := json.Marshal(payload)
			if err != nil {
				t.Fatalf("marshal gateway create payload: %v", err)
			}

			var got map[string]any
			if err := json.Unmarshal(encoded, &got); err != nil {
				t.Fatalf("unmarshal gateway create payload: %v", err)
			}
			want := map[string]any{
				"name":      name,
				"placement": tt.placement,
				"route":     `{"enabled":true}`,
				"oidc":      oidc,
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("gateway create payload = %#v, want %#v", got, want)
			}
		})
	}
}
