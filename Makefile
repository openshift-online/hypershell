CONTAINER_ENGINE?=$(shell command -v podman 2>/dev/null || echo docker)
LEFTHOOK_CMD=go tool lefthook

.PHONY: build-all
build-all:
	cd components/api-server && $(MAKE) image
	cd components/api-server && $(MAKE) image-controller

.PHONY: check-forbidden-terms
check-forbidden-terms:
	python3 scripts/check_forbidden_terms.py

.PHONY: check-dependency-pins
check-dependency-pins:
	python3 scripts/check_dependency_pins.py

.PHONY: check
check: check-forbidden-terms check-dependency-pins

.PHONY: hooks-install
hooks-install:
	$(LEFTHOOK_CMD) install

.PHONY: hooks-run
hooks-run:
	$(LEFTHOOK_CMD) run check

.PHONY: lint
lint: check
	cd components/api-server && go fmt ./... && go vet ./...
	cd components/control-plane && go fmt ./... && go vet ./...

.PHONY: test-all
test-all:
	cd components/api-server && $(MAKE) test

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
