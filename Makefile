CONTAINER_ENGINE?=$(shell command -v podman 2>/dev/null || echo docker)
LEFTHOOK_CMD=go tool lefthook
GO_TOOLCHAIN=go1.26.4
GOLANGCI_LINT_VERSION=v2.12.2
GOLANGCI_LINT_PACKAGE=github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)
DEPENDENCY_MIN_AGE_DAYS=14
PNPM_VERSION=11.15.1
PNPM?=pnpm
WEB_CONSOLE_IMAGE?=localhost/hypershell-web-console:dev

.PHONY: build-all
build-all:
	cd components/api-server && $(MAKE) image
	cd components/api-server && $(MAKE) image-controller
	$(CONTAINER_ENGINE) build -t $(WEB_CONSOLE_IMAGE) -f components/web-console/Dockerfile .

.PHONY: verify-pnpm
verify-pnpm:
	test "$$($(PNPM) --version)" = "$(PNPM_VERSION)"

.PHONY: install-js
install-js: verify-pnpm
	$(PNPM) install --frozen-lockfile

.PHONY: web-console-dev
web-console-dev: install-js
	$(PNPM) dev

.PHONY: web-console-image
web-console-image:
	$(CONTAINER_ENGINE) build -t $(WEB_CONSOLE_IMAGE) -f components/web-console/Dockerfile .

.PHONY: check-forbidden-terms
check-forbidden-terms:
	python3 scripts/check_forbidden_terms.py

.PHONY: check-dependency-pins
check-dependency-pins:
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

.PHONY: hooks-install
hooks-install:
	$(LEFTHOOK_CMD) install

.PHONY: hooks-run
hooks-run:
	$(LEFTHOOK_CMD) run check

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

.PHONY: lint-web-console
lint-web-console: install-js
	$(PNPM) --filter @openshift-online/hypershell-domain-probes check
	$(PNPM) --filter @openshift-online/hypershell-web-console check
	$(PNPM) --filter @openshift-online/hypershell-web-console-bff check

.PHONY: lint
lint: check install-js lint-api-server lint-control-plane lint-sdk-typescript lint-web-console

.PHONY: test-all
test-all: install-js
	cd components/api-server && $(MAKE) test
	$(PNPM) --filter @openshift-online/hypershell-domain-probes test:run
	$(PNPM) --filter @openshift-online/hypershell-web-console test:run
	$(PNPM) --filter @openshift-online/hypershell-web-console-bff test:run

.PHONY: kind-up
kind-up: build-all
	cd components/api-server && $(MAKE) kind-up

.PHONY: kind-down
kind-down:
	cd components/api-server && $(MAKE) kind-down

.PHONY: kind-rebuild
kind-rebuild:
	cd components/api-server && $(MAKE) kind-rebuild

.PHONY: kind-status
kind-status:
	cd components/api-server && $(MAKE) kind-status
