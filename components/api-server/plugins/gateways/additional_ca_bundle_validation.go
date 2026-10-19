package gateways

import (
	"strings"

	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/errors"
)

const maxAdditionalCABundleBytes = 256 * 1024

const (
	certificateBeginMarker = "-----BEGIN CERTIFICATE-----"
	certificateEndMarker   = "-----END CERTIFICATE-----"
)

// validateAdditionalCABundle validates the user-supplied PEM bundle without
// parsing certificate contents. OpenShell accepts self-signed destination CAs,
// so API validation only enforces safe PEM framing and rejects key material.
func validateAdditionalCABundle(bundle *string) *errors.ServiceError {
	if bundle == nil || *bundle == "" {
		return nil
	}
	if len(*bundle) > maxAdditionalCABundleBytes {
		return errors.Validation("additional_ca_bundle must not exceed 256 KiB")
	}
	if strings.TrimSpace(*bundle) == "" {
		return errors.Validation("additional_ca_bundle must contain at least one PEM certificate block")
	}
	if strings.Contains(*bundle, "PRIVATE KEY") {
		return errors.Validation("additional_ca_bundle must not contain private key material")
	}

	remaining := *bundle
	for {
		begin := strings.Index(remaining, "-----BEGIN")
		if begin == -1 {
			break
		}
		if !strings.HasPrefix(remaining[begin:], certificateBeginMarker) {
			return errors.Validation("additional_ca_bundle may contain only CERTIFICATE PEM blocks")
		}
		remaining = remaining[begin+len(certificateBeginMarker):]
	}

	remaining = *bundle
	certificateBlocks := 0
	for {
		begin := strings.Index(remaining, certificateBeginMarker)
		if begin == -1 {
			break
		}
		end := strings.Index(remaining[begin+len(certificateBeginMarker):], certificateEndMarker)
		if end == -1 {
			return errors.Validation("additional_ca_bundle must contain complete PEM certificate blocks")
		}
		certificateBlocks++
		remaining = remaining[begin+len(certificateBeginMarker)+end+len(certificateEndMarker):]
	}
	if certificateBlocks == 0 {
		return errors.Validation("additional_ca_bundle must contain complete PEM certificate blocks")
	}
	return nil
}
