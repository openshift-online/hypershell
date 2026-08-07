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
api_server_local=hypershell:dev
control_plane_local=hypershell-controller:dev
web_console_local=hypershell-web-console:dev

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
CLOUD_PROVIDER_KIND_VERSION?=v0.11.1
CERT_MANAGER_VERSION?=v1.21.1

# Kind config
KIND_CONFIG=deploy/kind/kind-config.yaml
KIND_DNS_PORT?=5553

# Service hostnames (routed through the networking Gateway)
API_HOSTNAME=api.hypershell.localhost
CONSOLE_HOSTNAME=console.hypershell.localhost
HEALTH_HOSTNAME=health.hypershell.localhost
KEYCLOAK_HOSTNAME=keycloak.hypershell.localhost

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
	@echo "    kind-up                  Create cluster + deploy all components"
	@echo "    kind-down                Delete cluster + stop cloud-provider-kind"
	@echo "    kind-status              Show cluster info, pods, services, swap state"
	@echo "    kind-deploy              Deploy into a new namespace (from branch name)"
	@echo "    kind-undeploy            Delete a namespace deployment"
	@echo "    kind-api-server-up       Build + swap API server from working tree"
	@echo "    kind-api-server-down     Revert API server to baseline image"
	@echo "    kind-control-plane-up    Build + swap control plane from working tree"
	@echo "    kind-control-plane-down  Revert control plane to baseline image"
	@echo "    kind-web-console-up      Hot reload (default) or build + swap web console"
	@echo "    kind-web-console-down    Revert web console to baseline image"
	@echo ""
	@echo "  Build"
	@echo "    build-all                Build all container images"
	@echo "    web-console-image        Build web console container image"
	@echo "    web-console-dev          Start web console dev server (pnpm)"
	@echo ""
	@echo "  Test & Lint"
	@echo "    test-all                 Run all test suites"
	@echo "    lint                     Run all linters (Go + JS/TS)"
	@echo "    lint-api-server          Lint API server (gofmt, go vet, golangci-lint)"
	@echo "    lint-control-plane       Lint control plane (gofmt, go vet, golangci-lint)"
	@echo "    lint-sdk-typescript      Lint TypeScript SDK"
	@echo "    lint-gateway-ui          Lint gateway UI package"
	@echo "    lint-web-console         Lint web console (app + BFF)"
	@echo ""
	@echo "  Policy"
	@echo "    check                    Run all policy checks"
	@echo "    check-forbidden-terms    Check for forbidden terms in source"
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

.PHONY: web-console-dev
web-console-dev: install-js
	$(PNPM) dev

.PHONY: web-console-image
web-console-image:
	$(CONTAINER_ENGINE) build -t $(web_console_local) \
		-f components/web-console/Dockerfile .

# ============================================================================
# Policy checks
# ============================================================================

.PHONY: check-forbidden-terms
check-forbidden-terms:
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

.PHONY: lint-gateway-ui
lint-gateway-ui: install-js
	$(PNPM) --filter @openshift-online/hypershell-domain-probes build
	$(PNPM) --filter @openshift-online/hypershell-gateway-ui check

.PHONY: lint-web-console
lint-web-console: install-js
	$(PNPM) --filter @openshift-online/hypershell-domain-probes check
	$(PNPM) --filter @openshift-online/hypershell-web-console check
	$(PNPM) --filter @openshift-online/hypershell-web-console-bff check

.PHONY: lint
lint: check install-js lint-api-server lint-control-plane lint-sdk-typescript lint-gateway-ui lint-web-console

# ============================================================================
# Test targets
# ============================================================================

.PHONY: test-all
test-all: install-js
	cd components/api-server && $(MAKE) test
	$(PNPM) --filter @openshift-online/hypershell-domain-probes test:run
	$(PNPM) --filter @openshift-online/hypershell-gateway-ui test:run
	$(PNPM) --filter @openshift-online/hypershell-web-console test:run
	$(PNPM) --filter @openshift-online/hypershell-web-console-bff test:run

# ============================================================================
# Kind cluster lifecycle — shell logic lives in scripts/kind/
# ============================================================================

export CONTAINER_ENGINE KIND_CLUSTER_NAME KIND_NAMESPACE
export KIND_HOT_RELOAD KIND_HOST_MOUNT_PATH KIND_KEYCLOAK_URL LOCAL_IMAGES
export KIND_PULL_SECRET
export GATEWAY_API_VERSION CLOUD_PROVIDER_KIND_VERSION CERT_MANAGER_VERSION
export IMAGE_REGISTRY IMAGE_TAG KIND_CONFIG
export api_server_ref control_plane_ref web_console_ref
export api_server_local control_plane_local web_console_local
export build_version build_time
export API_HOSTNAME CONSOLE_HOSTNAME HEALTH_HOSTNAME KEYCLOAK_HOSTNAME
export KIND_DNS_PORT

.PHONY: kind-up
kind-up:
	@scripts/kind/up.sh

.PHONY: kind-down
kind-down:
	@scripts/kind/down.sh

.PHONY: kind-status
kind-status:
	@scripts/kind/status.sh

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

.PHONY: kind-deploy
kind-deploy:
	@scripts/kind/deploy-namespace.sh

.PHONY: kind-undeploy
kind-undeploy:
	@scripts/kind/deploy-namespace.sh undeploy
