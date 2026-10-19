package gateways

import (
	"encoding/json"
	"strings"
	"testing"
)

const testAdditionalCACertificate = `-----BEGIN CERTIFICATE-----
MIIDATCCAemgAwIBAgIUYj2DnFBGhdltTJY8x3LhI3wKRIAwDQYJKoZIhvcNAQEL
BQAwEDEOMAwGA1UEAwwFdGVzdENBMB4XDTI2MDgyNTAwMDAwMFoXDTI2MDgyNjAw
MDAwMFowEDEOMAwGA1UEAwwFdGVzdENBMIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8A
MIIBCgKCAQEAu31KUYP2SmkNs3KR/WNmf4EPmD7lA8aZP05pYW5TcvJZz44iL7xh
w8WwsFO1Ml0cuSB9yODa7PQdLS19qc1zntgM1YebRz+7g6K8K8fO2wdTpw8rSLO1
OKs7CkTTRFIIWiQLYf3Dh0cKQtRz1r1t2p6/03TzATZSuIbmo1vMpPzV9iH4DOko
CbDLRk0Mx5qmGgEMkzAq3sWfe0qCE62ag00jJLc6tB9slQZSJ6q2VP+Crq7vJLSV
wqvZs8l7RnwqhTBxqcpSYI9wTKlKYbz7u9MEoWrFxIQBsdRfDPzz+OoBkx6NSx20
Bt5sqY/lIdFWyCzBc4OqLG9Sl+iNYzz5cwIDAQABo1MwUTAdBgNVHQ4EFgQUxEzp
0r1qJzLmlOB29QKrRXfCUMgwHwYDVR0jBBgwFoAUxEzp0r1qJzLmlOB29QKrRXfC
UMgwDwYDVR0TAQH/BAUwAwEB/zANBgkqhkiG9w0BAQsFAAOCAQEAJCG94k6MsfFX
uDJ9Wf1H6eYiC4E4BR7DLPk5go85JXKT2vN/hXO5MTFY6sUNVAsNjwZJd0uDQo/B
CNFhNqE0aUTKq4QF+kiREpBIsz1On4VGCIwbPS0YHnxiBzaJSXCySa/BI0AHDYl1
NUmoiwLxkXyOf2N+WLOpyXD33YHJa6O5MWyDJTTZgZi3M/WlwdlK6waFiR80qlyp
mbJhh9gHLoP9YRQnlQPFBlMF1uUmDBQ95m2WhMgsWVnMkFRpGquB6Uq0sZrL6wDd
UvuUpdtkTEv+uQXKN9L1Z1oKsTf6Wzgk2sBiyocLh5OhA42XkEfPt7Hegw2JkE7j
TuOT0WoZNw==
-----END CERTIFICATE-----`

func TestGatewayPatchRequestAdditionalCABundleNullClearsBundle(t *testing.T) {
	var patch gatewayPatchRequest
	if err := json.Unmarshal([]byte(`{"additional_ca_bundle":null}`), &patch); err != nil {
		t.Fatalf("unmarshal patch: %v", err)
	}
	if !patch.additionalCABundleSet {
		t.Fatal("additional_ca_bundle should be marked set when JSON null is supplied")
	}
	if patch.AdditionalCABundle == nil || *patch.AdditionalCABundle != "" {
		t.Fatalf("AdditionalCaBundle = %v, want pointer to empty string", patch.AdditionalCABundle)
	}

	if err := json.Unmarshal([]byte(`{"name":"renamed"}`), &patch); err != nil {
		t.Fatalf("unmarshal patch without bundle: %v", err)
	}
	if patch.additionalCABundleSet {
		t.Fatal("additional_ca_bundle should not be marked set when omitted")
	}
}

func TestValidateAdditionalCABundle(t *testing.T) {
	tooLarge := testAdditionalCACertificate + strings.Repeat("x", maxAdditionalCABundleBytes)
	privateKey := "-----BEGIN PRIVATE KEY-----\nnot-a-key\n-----END PRIVATE KEY-----"

	tests := []struct {
		name       string
		bundle     string
		wantReason string
	}{
		{name: "valid single certificate", bundle: testAdditionalCACertificate},
		{name: "valid multiple certificates", bundle: testAdditionalCACertificate + "\n" + testAdditionalCACertificate},
		{name: "empty removes bundle", bundle: ""},
		{name: "whitespace only", bundle: " \n\t ", wantReason: "must contain at least one PEM certificate block"},
		{name: "DER without PEM", bundle: "0\x82\x03\x010\x82", wantReason: "must contain complete PEM certificate blocks"},
		{name: "private key", bundle: testAdditionalCACertificate + "\n" + privateKey, wantReason: "must not contain private key material"},
		{name: "mixed PEM blocks", bundle: testAdditionalCACertificate + "\n-----BEGIN PUBLIC KEY-----\ndata\n-----END PUBLIC KEY-----", wantReason: "may contain only CERTIFICATE PEM blocks"},
		{name: "too large", bundle: tooLarge, wantReason: "must not exceed 256 KiB"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			bundle := tc.bundle
			err := validateAdditionalCABundle(&bundle)
			if tc.wantReason == "" {
				if err != nil {
					t.Fatalf("validateAdditionalCABundle() error = %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("validateAdditionalCABundle() error = nil")
			}
			if !strings.Contains(err.Error(), "additional_ca_bundle") || !strings.Contains(err.Error(), tc.wantReason) {
				t.Errorf("validation error = %q, want field and reason %q", err.Error(), tc.wantReason)
			}
			if strings.Contains(err.Error(), tc.bundle) || strings.Contains(err.Error(), "not-a-key") {
				t.Errorf("validation error must not echo bundle contents: %q", err.Error())
			}
		})
	}
}
