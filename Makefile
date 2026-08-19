CONTAINER_ENGINE?=$(shell command -v podman 2>/dev/null || echo docker)
LEFTHOOK_CMD=go tool lefthook
GO_TOOLCHAIN=go1.26.4
GOLANGCI_LINT_VERSION=v2.12.2
GOLANGCI_LINT_PACKAGE=github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)
DEPENDENCY_MIN_AGE_DAYS=14
PNPM_MIN_VERSION=11.15.1
PNPM?=pnpm

# --- Image registry and tags ---
IMAGE_REGISTRY?=quay.io/redhat-services-prod/hcm-eng-prod-tenant/hypershell-main
IMAGE_TAG?=latest

# Build version (embedded in api-server binary via ldflags)
git_sha:=$(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
git_dirty:=$(shell git diff --quiet 2>/dev/null || echo -modified)
build_version:=$(git_sha)$(git_dirty)
build_time:=$(shell date -u '+%Y-%m-%d %H:%M:%S UTC')

# Computed baseline references (registry images used in Kind manifests)
api_server_ref=$(IMAGE_REGISTRY)/hypershell-api-server-main:$(IMAGE_TAG)
control_plane_ref=$(IMAGE_REGISTRY)/hypershell-control-plane-main:$(IMAGE_TAG)
web_console_ref=$(IMAGE_REGISTRY)/hypershell-web-console-main:$(IMAGE_TAG)

# Local dev image names
api_server_local=localhost/hypershell:dev
control_plane_local=localhost/hypershell-controller:dev
web_console_local=localhost/hypershell-web-console:dev

# --- Kind cluster configuration ---
KIND_CLUSTER_NAME?=hypershell-dev
KIND_NAMESPACE?=hypershell-system
KIND_HOT_RELOAD?=true
KIND_HOST_MOUNT_PATH?=$(shell git rev-parse --show-toplevel 2>/dev/null || pwd)
KIND_KEYCLOAK_URL?=
LOCAL_IMAGES?=
KIND_PULL_SECRET?=

# Prerequisite versions
GATEWAY_API_VERSION?=v1.5.1
# kind v0.32.0 has a podman 6+ ListClusters bug (kubernetes-sigs/kind#4231).
# Pin to a main commit that includes the fix (kubernetes-sigs/kind#4203) until
# the next kind release ships it.  go install uses Go pseudo-versions.
KIND_VERSION?=v0.32.1-0.20260811083914-7650cab268f5
# Build from fork with BackendTLSPolicy support until upstreamed. Pin an exact
# commit (mirrors the KIND_VERSION pseudo-version pin above) so every developer
# builds the same deterministic binary instead of tracking the moving branch
# tip. CLOUD_PROVIDER_KIND_REF is the tip of branch `hypershell` at pin time;
# bump it when the fork advances. The binary auto-stamps this commit as
# vcs.revision, which kind-prereqs uses to skip rebuilds that are already current.
CLOUD_PROVIDER_KIND_REPO?=https://github.com/squizzi/cloud-provider-kind.git
CLOUD_PROVIDER_KIND_REF?=08ce4ea4cc10bce8ffbcf4f859a086bb6b292230
# Optional testing override: build from a branch tip (or any git ref) instead of
# the pinned REF, e.g. `CLOUD_PROVIDER_KIND_BRANCH=my-experiment make kind-up`.
# Empty by default so normal builds stay deterministic and idempotent-by-SHA.
# When set, kind-prereqs always rebuilds from that ref and records the commit it
# actually resolved to, so up.sh restarts to pick up a moved branch tip.
CLOUD_PROVIDER_KIND_BRANCH?=
CERT_MANAGER_VERSION?=v1.21.1
CNPG_VERSION?=v1.30.0
AGENT_SANDBOX_VERSION?=v0.5.4

# PostgreSQL image for API server CNPG cluster (unset = CNPG default)
HYPERSHELL_DATABASE_IMAGE?=

# Kind config
KIND_CONFIG=deploy/kind/kind-config.yaml
KIND_DNS_PORT?=5553

# Service hostnames (routed through the networking Gateway)
API_HOSTNAME=api.hypershell.localhost
CONSOLE_HOSTNAME=console.hypershell.localhost
HEALTH_HOSTNAME=health.hypershell.localhost
KEYCLOAK_HOSTNAME=keycloak.hypershell.localhost
KEYCLOAK_OIDC_ISSUER?=https://$(KEYCLOAK_HOSTNAME)/realms/hypershell

# ============================================================================
# Help
# ============================================================================

.PHONY: help
help:
	@echo ""
	@echo "  HyperShell Makefile"
	@echo "  ==================="
	@echo ""
	@echo "  Local Development (Kind)"
	@echo "    All targets operate on KIND_NAMESPACE (default: hypershell-system)."
	@echo ""
	@echo "    kind-env                 Print environment variables for local setup"
	@echo "    kind-up                  Create cluster + deploy all components (OIDC enabled)"
	@echo "                             LOCAL_IMAGES=true: build from working tree (default)"
	@echo "                             LOCAL_IMAGES=true BUILD_SOURCE=baseline: build from origin/main"
	@echo "    kind-down                Remove namespace and its resources"
	@echo "    kind-teardown            Destroy Kind cluster, stop cloud-provider-kind"
	@echo "    kind-status              Show cluster info, pods, services, swap state"
	@echo "    kind-fix-ports           Re-establish host port forwarding (443 + 8080)"
	@echo "    kind-api-server-up       Build + swap API server from working tree"
	@echo "    kind-api-server-down     Revert API server to baseline image"
	@echo "    kind-control-plane-up    Build + swap control plane from working tree"
	@echo "    kind-control-plane-down  Revert control plane to baseline image"
	@echo "    kind-web-console-up      Hot reload (default) or build + swap web console (KIND_HOT_RELOAD=false)"
	@echo "    kind-web-console-down    Revert web console to baseline image"
	@echo "    kind-gateway-trust       Print SSL_CERT_FILE export so the openshell CLI trusts the dev CA"
	@echo ""
	@echo "  Build"
	@echo "    build-all                Build all container images"
	@echo "    build-api-server         Build API server container image"
	@echo "    build-cli                Build CLI binary"
	@echo "    build-controller         Build control plane container image"
	@echo "    build-web-console        Build web console container image"
	@echo ""
	@echo "  Test & Lint"
	@echo "    test-all                 Run all test suites"
	@echo "    e2e                      Run E2E tests locally (requires Kind cluster)"
	@echo "    lint                     Run all linters (Go + JS/TS)"
	@echo "    lint-api-server          Lint API server (gofmt, go vet, golangci-lint)"
	@echo "    lint-cli                 Lint CLI (gofmt, go vet, golangci-lint)"
	@echo "    lint-control-plane       Lint control plane (gofmt, go vet, golangci-lint)"
	@echo "    lint-sdk-typescript      Lint TypeScript SDK"
	@echo "    lint-gateway-management-ui  Lint gateway management UI package"
	@echo "    lint-web-console         Lint web console (app + BFF)"
	@echo ""
	@echo "  Policy"
	@echo "    check                    Run all policy checks"
	@echo "    check-forbidden-terms    Check for forbidden or discouraged text"
	@echo "    check-dependency-pins    Verify dependency version pins"
	@echo "    check-dependency-age     Verify dependency minimum age"
	@echo "    check-ci-components      Verify CI component registration"
	@echo ""
	@echo "  Hooks"
	@echo "    hooks-install            Install Git hooks (lefthook)"
	@echo "    hooks-run                Run hook checks manually"
	@echo ""

# ============================================================================
# Build targets
# ============================================================================

.PHONY: build-all
build-all:
	@scripts/kind/build-images.sh

.PHONY: verify-pnpm
verify-pnpm:
	@current=$$($(PNPM) --version); \
	printf '%s\n%s\n' "$(PNPM_MIN_VERSION)" "$$current" | sort -V -C || \
	  { echo "pnpm $$current < minimum $(PNPM_MIN_VERSION)"; exit 1; }

.PHONY: install-js
install-js: verify-pnpm
	$(PNPM) install --frozen-lockfile

.PHONY: build-api-server
build-api-server:
	$(CONTAINER_ENGINE) build -t $(api_server_local) \
		--build-arg GIT_VERSION=$(build_version) --build-arg BUILD_TIME="$(build_time)" \
		components/api-server

.PHONY: build-controller
build-controller:
	$(CONTAINER_ENGINE) build -t $(control_plane_local) \
		-f components/control-plane/Dockerfile .

.PHONY: build-cli
build-cli:
	cd components/cli && CGO_ENABLED=0 go build -ldflags="-s -w" -o hsctl ./cmd/hypershell

.PHONY: build-web-console
build-web-console:
	$(CONTAINER_ENGINE) build -t $(web_console_local) \
		-f components/web-console/Dockerfile .

# ============================================================================
# Policy checks
# ============================================================================

.PHONY: test-forbidden-terms-policy
test-forbidden-terms-policy:
	PYTHONDONTWRITEBYTECODE=1 python3 -m unittest scripts/test_check_forbidden_terms.py

.PHONY: check-forbidden-terms
check-forbidden-terms: test-forbidden-terms-policy
	python3 scripts/check_forbidden_terms.py

.PHONY: test-dependency-pin-policy
test-dependency-pin-policy:
	PYTHONDONTWRITEBYTECODE=1 python3 -m unittest scripts/test_check_dependency_pins.py

.PHONY: check-dependency-pins
check-dependency-pins: test-dependency-pin-policy
	python3 scripts/check_dependency_pins.py

.PHONY: check-ci-components
check-ci-components:
	python3 scripts/check_ci_components.py

.PHONY: test-dependency-age-policy
test-dependency-age-policy:
	PYTHONDONTWRITEBYTECODE=1 python3 -m unittest scripts/test_check_dependency_age.py

.PHONY: check-dependency-age
check-dependency-age: test-dependency-age-policy
	PYTHONDONTWRITEBYTECODE=1 python3 scripts/check_dependency_age.py --min-age-days $(DEPENDENCY_MIN_AGE_DAYS)

.PHONY: check
check: check-forbidden-terms check-dependency-pins check-ci-components check-dependency-age

# ============================================================================
# Git hooks
# ============================================================================

.PHONY: hooks-install
hooks-install:
	$(LEFTHOOK_CMD) install

.PHONY: hooks-run
hooks-run:
	$(LEFTHOOK_CMD) run check

# ============================================================================
# Lint targets
# ============================================================================

.PHONY: lint-api-server
lint-api-server:
	@unformatted="$$(gofmt -l components/api-server)"; \
	if [ -n "$$unformatted" ]; then \
		echo "The following API server files are not formatted:"; \
		echo "$$unformatted"; \
		exit 1; \
	fi
	cd components/api-server && GOTOOLCHAIN=$(GO_TOOLCHAIN) go vet ./...
	cd components/api-server && GOTOOLCHAIN=$(GO_TOOLCHAIN) go run $(GOLANGCI_LINT_PACKAGE) run --timeout=5m

.PHONY: lint-cli
lint-cli:
	@unformatted="$$(gofmt -l components/cli)"; \
	if [ -n "$$unformatted" ]; then \
		echo "The following CLI files are not formatted:"; \
		echo "$$unformatted"; \
		exit 1; \
	fi
	cd components/cli && GOTOOLCHAIN=$(GO_TOOLCHAIN) go vet ./...
	cd components/cli && GOTOOLCHAIN=$(GO_TOOLCHAIN) go run $(GOLANGCI_LINT_PACKAGE) run --timeout=5m

.PHONY: lint-control-plane
lint-control-plane:
	@unformatted="$$(gofmt -l components/control-plane)"; \
	if [ -n "$$unformatted" ]; then \
		echo "The following control plane files are not formatted:"; \
		echo "$$unformatted"; \
		exit 1; \
	fi
	cd components/control-plane && GOTOOLCHAIN=$(GO_TOOLCHAIN) go vet ./...
	cd components/control-plane && GOTOOLCHAIN=$(GO_TOOLCHAIN) go run $(GOLANGCI_LINT_PACKAGE) run --timeout=5m

.PHONY: lint-sdk-typescript
lint-sdk-typescript: install-js
	$(PNPM) --filter @openshift-online/hypershell-sdk check

.PHONY: lint-gateway-management-ui
lint-gateway-management-ui: install-js
	$(PNPM) --filter @openshift-online/hypershell-domain-probes build
	$(PNPM) --filter @openshift-online/hypershell-gateway-management-ui check

.PHONY: lint-web-console
lint-web-console: install-js
	$(PNPM) --filter @openshift-online/hypershell-domain-probes check
	$(PNPM) --filter @openshift-online/hypershell-web-console check
	$(PNPM) --filter @openshift-online/hypershell-web-console-bff check

.PHONY: lint
lint: check install-js lint-api-server lint-cli lint-control-plane lint-sdk-typescript lint-gateway-management-ui lint-web-console

# ============================================================================
# Test targets
# ============================================================================

.PHONY: test-all
test-all: install-js
	cd components/api-server && $(MAKE) test
	$(PNPM) --filter @openshift-online/hypershell-domain-probes test:run
	$(PNPM) --filter @openshift-online/hypershell-gateway-management-ui test:run
	$(PNPM) --filter @openshift-online/hypershell-web-console test:run
	$(PNPM) --filter @openshift-online/hypershell-web-console-bff test:run

# ============================================================================
# Kind cluster lifecycle - shell logic lives in scripts/kind/
# ============================================================================

export CONTAINER_ENGINE KIND_CLUSTER_NAME KIND_NAMESPACE
export KIND_HOT_RELOAD KIND_HOST_MOUNT_PATH KIND_KEYCLOAK_URL LOCAL_IMAGES BUILD_SOURCE
export KIND_PULL_SECRET
export GATEWAY_API_VERSION KIND_VERSION CLOUD_PROVIDER_KIND_REPO CLOUD_PROVIDER_KIND_REF CLOUD_PROVIDER_KIND_BRANCH CERT_MANAGER_VERSION CNPG_VERSION AGENT_SANDBOX_VERSION
export HYPERSHELL_DATABASE_IMAGE
export IMAGE_REGISTRY IMAGE_TAG KIND_CONFIG
export api_server_ref control_plane_ref web_console_ref
export API_SERVER_IMAGE CONTROL_PLANE_IMAGE WEB_CONSOLE_IMAGE
export api_server_local control_plane_local web_console_local
export build_version build_time
export API_HOSTNAME CONSOLE_HOSTNAME HEALTH_HOSTNAME KEYCLOAK_HOSTNAME KEYCLOAK_OIDC_ISSUER
export KIND_DNS_PORT

# Build cloud-provider-kind from a fork that adds BackendTLSPolicy support
# (TLS re-encryption to backends).  The fork also bundles the podman 6+ kind
# fix, so no go mod replace is needed.
#
# Idempotent by commit: the Go binary auto-stamps its source revision, so we
# compare the on-disk vcs.revision against the pinned CLOUD_PROVIDER_KIND_REF and
# only rebuild when it is missing or out of date. The pinned commit is fetched
# directly (GitHub allows fetch-by-SHA), guaranteeing a deterministic build.
# bin/.cloud-provider-kind.sha records the built commit so up.sh can decide
# whether a running cloud-provider-kind needs restarting -- without invoking go.
.PHONY: kind-prereqs
kind-prereqs:
	@mkdir -p bin
	@ref="$(CLOUD_PROVIDER_KIND_REF)"; \
	branch="$(CLOUD_PROVIDER_KIND_BRANCH)"; \
	if [ -n "$$branch" ]; then \
	  fetchref="$$branch"; \
	  echo "==> Building cloud-provider-kind from ref '$$branch' (CLOUD_PROVIDER_KIND_BRANCH override) -> bin/cloud-provider-kind"; \
	else \
	  fetchref="$$ref"; \
	  if [ -x bin/cloud-provider-kind ]; then \
	    ondisk="$$(go version -m bin/cloud-provider-kind 2>/dev/null | awk -F= '/[[:space:]]vcs.revision=/{print $$2}')"; \
	    if [ "$$ondisk" = "$$ref" ]; then \
	      echo "==> bin/cloud-provider-kind already at $$ref, skipping build"; \
	      printf '%s\n' "$$ref" > bin/.cloud-provider-kind.sha; \
	      exit 0; \
	    fi; \
	    echo "==> bin/cloud-provider-kind is $${ondisk:-unknown}, rebuilding at $$ref"; \
	  else \
	    echo "==> Building cloud-provider-kind@$$ref -> bin/cloud-provider-kind"; \
	  fi; \
	fi; \
	tmpdir=$$(mktemp -d) && \
	  git init -q "$$tmpdir" && \
	  git -C "$$tmpdir" remote add origin $(CLOUD_PROVIDER_KIND_REPO) && \
	  git -C "$$tmpdir" fetch -q --depth 1 origin "$$fetchref" && \
	  git -C "$$tmpdir" -c advice.detachedHead=false checkout -q FETCH_HEAD && \
	  built="$$(git -C "$$tmpdir" rev-parse HEAD)" && \
	  ( cd "$$tmpdir" && CGO_ENABLED=0 go build -o $(CURDIR)/bin/cloud-provider-kind . ) && \
	  rm -rf "$$tmpdir" && \
	  printf '%s\n' "$$built" > bin/.cloud-provider-kind.sha && \
	  echo "==> Done - binary in ./bin/cloud-provider-kind ($$built)"

.PHONY: kind-env
kind-env:
	@echo "# Environment variables for local Kind cluster setup"
	@echo "# Copy the export block below to run 'make kind-up' with these settings:"
	@echo ""
	@echo "export CONTAINER_ENGINE=$(CONTAINER_ENGINE)"
	@echo "export KIND_CLUSTER_NAME=$(KIND_CLUSTER_NAME)"
	@echo "export KIND_NAMESPACE=$(KIND_NAMESPACE)"
	@echo "export KIND_HOT_RELOAD=$(KIND_HOT_RELOAD)"
	@echo "export KIND_HOST_MOUNT_PATH=$(KIND_HOST_MOUNT_PATH)"
	@echo "export KIND_KEYCLOAK_URL=$(KIND_KEYCLOAK_URL)"
	@echo "export LOCAL_IMAGES=$(LOCAL_IMAGES)"
	@echo "export KIND_PULL_SECRET=$(KIND_PULL_SECRET)"
	@echo "export KIND_DB_IMAGE=$(KIND_DB_IMAGE)"
	@echo "export GATEWAY_API_VERSION=$(GATEWAY_API_VERSION)"
	@echo "export KIND_VERSION=$(KIND_VERSION)"
	@echo "export CLOUD_PROVIDER_KIND_REPO=$(CLOUD_PROVIDER_KIND_REPO)"
	@echo "export CLOUD_PROVIDER_KIND_REF=$(CLOUD_PROVIDER_KIND_REF)"
	@echo "export CLOUD_PROVIDER_KIND_BRANCH=$(CLOUD_PROVIDER_KIND_BRANCH)"
	@echo "export CERT_MANAGER_VERSION=$(CERT_MANAGER_VERSION)"
	@echo "export AGENT_SANDBOX_VERSION=$(AGENT_SANDBOX_VERSION)"
	@echo "export IMAGE_REGISTRY=$(IMAGE_REGISTRY)"
	@echo "export IMAGE_TAG=$(IMAGE_TAG)"
	@echo "export KIND_CONFIG=$(KIND_CONFIG)"
	@echo "export API_HOSTNAME=$(API_HOSTNAME)"
	@echo "export CONSOLE_HOSTNAME=$(CONSOLE_HOSTNAME)"
	@echo "export HEALTH_HOSTNAME=$(HEALTH_HOSTNAME)"
	@echo "export KEYCLOAK_HOSTNAME=$(KEYCLOAK_HOSTNAME)"
	@echo "export KEYCLOAK_OIDC_ISSUER=$(KEYCLOAK_OIDC_ISSUER)"
	@echo "export KIND_DNS_PORT=$(KIND_DNS_PORT)"
	@echo "export API_SERVER_IMAGE=$(API_SERVER_IMAGE)"
	@echo "export CONTROL_PLANE_IMAGE=$(CONTROL_PLANE_IMAGE)"
	@echo "export WEB_CONSOLE_IMAGE=$(WEB_CONSOLE_IMAGE)"

.PHONY: kind-up
kind-up:
	@scripts/kind/up.sh

.PHONY: kind-down
kind-down:
	@scripts/kind/down.sh

.PHONY: kind-teardown
kind-teardown:
	@scripts/kind/teardown.sh

.PHONY: kind-status
kind-status:
	@scripts/kind/status.sh

.PHONY: kind-fix-ports
kind-fix-ports:
	@scripts/kind/port-forward.sh

.PHONY: kind-api-server-up
kind-api-server-up:
	@scripts/kind/swap-component.sh up api-server

.PHONY: kind-api-server-down
kind-api-server-down:
	@scripts/kind/swap-component.sh down api-server

.PHONY: kind-control-plane-up
kind-control-plane-up:
	@scripts/kind/swap-component.sh up control-plane

.PHONY: kind-control-plane-down
kind-control-plane-down:
	@scripts/kind/swap-component.sh down control-plane

.PHONY: kind-web-console-up
kind-web-console-up:
	@scripts/kind/swap-component.sh up web-console

.PHONY: kind-web-console-down
kind-web-console-down:
	@scripts/kind/swap-component.sh down web-console

.PHONY: kind-gateway-trust
kind-gateway-trust:
	@scripts/kind/gateway-trust.sh

generate-cli:
	cd scripts/cli-generator && go run . \
		--spec ../../components/api-server/openapi/openapi.yaml \
		--out ../../components/cli \
		--binary hypershell \
		--project hypershell \
		--api-prefix /api/hypershell/v1 \
		--module github.com/openshift-online/hypershell/components/cli

generate-sdk-go:
	cd scripts/sdk-generator && go run . \
		--spec ../../components/api-server/openapi/openapi.yaml \
		--go-out ../../components/sdk-go \
		--module github.com/openshift-online/hypershell/components/sdk-go \
		--ts-out ../../components/sdk-typescript
# ============================================================================
# E2E Tests
# ============================================================================

.PHONY: e2e
e2e:
	@echo ""
	@echo "==> Running E2E tests (Kind)"
	@echo ""
	@E2E_INFRA_DRIVER=kind \
		E2E_PROVISION_TIMEOUT=300 \
		E2E_SANDBOX_TIMEOUT=180 \
		bash tests/e2e/e2e-openshell.sh

# Browser-driven end-to-end trace verification (WEB-TRACE-10). Requires a Kind
# cluster brought up with tracing enabled (KIND_JAEGER=true make kind-up), so
# Jaeger is deployed and the web-console BFF exports to it.
.PHONY: e2e-tracing
e2e-tracing:
	@echo ""
	@echo "==> Verifying end-to-end traces reach Jaeger (Kind)"
	@echo "    (requires: KIND_JAEGER=true make kind-up)"
	@echo ""
	@pnpm --filter @openshift-online/hypershell-web-console test:e2e:live
