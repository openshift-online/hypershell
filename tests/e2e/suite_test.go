package e2e

import (
	"os"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/openshift-online/hypershell/tests/e2e/driver"
	"github.com/openshift-online/hypershell/tests/e2e/harness"
)

// TestE2E is the single go test entry point for the functional suite. It builds
// the shared Kubernetes clients, resolves the E2EInfraDriver from the KUBECONFIG
// context (or E2E_INFRA_DRIVER), and runs the ordered phase steps as named
// subtests of E2ESuite. There is no wrapper shell script; `make e2e` invokes this.
func TestE2E(t *testing.T) {
	clients, err := harness.NewClients()
	if err != nil {
		t.Fatalf("build Kubernetes clients from KUBECONFIG context: %v", err)
	}

	d, err := driver.Resolve(t.Context(), clients, os.Getenv("E2E_INFRA_DRIVER"))
	if err != nil {
		// Unknown driver and auto-detection failures fail fast with a non-zero exit.
		t.Fatalf("resolve infra driver: %v", err)
	}
	t.Logf("resolved infra driver: %s", d.Name())

	suite.Run(t, newE2ESuite(d, clients))
}
