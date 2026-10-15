package gateways

import "testing"

func strptr(s string) *string { return &s }

func TestGatewayImagePinned(t *testing.T) {
	cases := []struct {
		name  string
		image *string
		want  bool
	}{
		{"nil image", nil, false},
		{"empty image", strptr(""), false},
		{"whitespace image", strptr("   "), false},
		{"pinned image", strptr("quay.io/acme/gateway:v1"), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := gatewayImagePinned(&Gateway{Image: tc.image}); got != tc.want {
				t.Fatalf("gatewayImagePinned(%v) = %v, want %v", tc.image, got, tc.want)
			}
		})
	}
}

func TestPresentGatewayVersionFields(t *testing.T) {
	gw := &Gateway{Image: strptr("quay.io/acme/gateway:v1")}
	gw.ID = "g-1"

	present := PresentGateway(gw, "", true, true, "v0.1.2-rhaiv.7")
	if present.ProvisioningVersion == nil || *present.ProvisioningVersion != "v0.1.2-rhaiv.7" {
		t.Fatalf("ProvisioningVersion = %v, want v0.1.2-rhaiv.7", present.ProvisioningVersion)
	}
	if present.ImagePinned == nil || !*present.ImagePinned {
		t.Fatalf("ImagePinned = %v, want true", present.ImagePinned)
	}

	// Empty provisioning version is omitted, and an unset image is not pinned.
	none := PresentGateway(&Gateway{}, "", true, true, "")
	if none.ProvisioningVersion != nil {
		t.Fatalf("ProvisioningVersion = %v, want nil", none.ProvisioningVersion)
	}
	if none.ImagePinned == nil || *none.ImagePinned {
		t.Fatalf("ImagePinned = %v, want false", none.ImagePinned)
	}
}
