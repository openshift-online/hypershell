BINARY_NAME=hypershell
CONTAINER_ENGINE?=$(shell command -v podman 2>/dev/null || echo docker)
POSTGRES_IMAGE=postgres:18@sha256:a02db8cac496f15b094798a38254f14d6e00741f709360e5e00bb6668ea31636
OPENAPI_SPEC=$(abspath openapi/openapi.yaml)
SDK_GENERATOR_DIR=$(abspath ../../scripts/sdk-generator)
SDK_TYPESCRIPT_DIR=$(abspath ../sdk-typescript)
SDK_API_PREFIX=/api/hypershell/v1

# Version information for ldflags
git_sha:=$(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
git_dirty:=$(shell git diff --quiet 2>/dev/null || echo "-modified")
build_version:=$(git_sha)$(git_dirty)
build_time:=$(shell date -u '+%Y-%m-%d %H:%M:%S UTC')
ldflags=-X github.com/openshift-online/hypershell/components/api-server/pkg/api.Version=$(build_version) -X 'github.com/openshift-online/hypershell/components/api-server/pkg/api.BuildTime=$(build_time)'

.PHONY: binary
binary:
	@echo "Building version: $(build_version)"
	go build -ldflags="$(ldflags)" -o $(BINARY_NAME) ./cmd/hypershell

.PHONY: install
install:
	go install -ldflags="$(ldflags)" ./cmd/hypershell

.PHONY: run
run: binary
	./$(BINARY_NAME) migrate
	./$(BINARY_NAME) serve

.PHONY: run-no-auth
run-no-auth: binary
	./$(BINARY_NAME) migrate
	./$(BINARY_NAME) serve --enable-authz=false --enable-jwt=false

.PHONY: test
test: install
	go test -v ./...

.PHONY: test-integration
test-integration: install
	API_ENV=integration_testing go test -p 1 -v ./plugins/... -count=1

.PHONY: clean
clean:
	rm -f $(BINARY_NAME)

.PHONY: generate
generate:
	rm -rf pkg/api/openapi
	$(CONTAINER_ENGINE) build -t $(BINARY_NAME)-openapi -f Dockerfile.openapi .
	id=$$($(CONTAINER_ENGINE) create $(BINARY_NAME)-openapi) && \
	$(CONTAINER_ENGINE) cp $$id:/local/pkg/api/openapi ./pkg/api/openapi && \
	$(CONTAINER_ENGINE) rm $$id

.PHONY: generate-sdk generate-sdk-ts generate-sdk-typescript
generate-sdk: generate-sdk-ts
generate-sdk-typescript: generate-sdk-ts

generate-sdk-ts:
	test -f "$(SDK_GENERATOR_DIR)/go.mod"
	rm -rf "$(SDK_TYPESCRIPT_DIR)/src"
	cd "$(SDK_GENERATOR_DIR)" && GOWORK=off go run . \
		--spec "$(OPENAPI_SPEC)" \
		--ts-out "$(SDK_TYPESCRIPT_DIR)" \
		--api-prefix "$(SDK_API_PREFIX)" \
		--project hypershell
	find "$(SDK_TYPESCRIPT_DIR)/src" -type f -name '*.ts' -exec sed -i \
		-e 's|^// Source: .*|// Source: components/api-server/openapi/openapi.yaml|' \
		-e '\|^// Generated:|d' {} +

.PHONY: db/setup
db/setup:
	$(CONTAINER_ENGINE) run -d --name hypershell-postgres \
		-p 5432:5432 \
		-e POSTGRES_PASSWORD=postgres \
		-e POSTGRES_USER=postgres \
		-e POSTGRES_DB=hypershell \
		$(POSTGRES_IMAGE)

.PHONY: db/teardown
db/teardown:
	$(CONTAINER_ENGINE) stop hypershell-postgres || true
	$(CONTAINER_ENGINE) rm hypershell-postgres || true

.PHONY: db/login
db/login:
	$(CONTAINER_ENGINE) exec -it hypershell-postgres psql -U postgres -d hypershell

.PHONY: proto
proto:
	cd proto && buf generate

.PHONY: proto-clean
proto-clean:
	rm -rf pkg/api/grpc/

KIND_CLUSTER_NAME?=hypershell-dev
KIND_API_PORT?=23080
IMAGE_NAME=hypershell
CONTROLLER_IMAGE_NAME=hypershell-controller
IMAGE_TAG=dev
REPO_ROOT=$(shell git rev-parse --show-toplevel)

.PHONY: image
image:
	$(CONTAINER_ENGINE) build -t $(IMAGE_NAME):$(IMAGE_TAG) \
		-f Dockerfile \
		--build-arg GIT_VERSION=$(build_version) \
		--build-arg BUILD_TIME="$(build_time)" \
		.

.PHONY: image-controller
image-controller:
	$(CONTAINER_ENGINE) build -t $(CONTROLLER_IMAGE_NAME):$(IMAGE_TAG) \
		-f $(REPO_ROOT)/components/control-plane/Dockerfile \
		$(REPO_ROOT)

.PHONY: images
images: image image-controller

.PHONY: kind-up
kind-up: images
	kind create cluster --name $(KIND_CLUSTER_NAME) --config deploy/kind/kind-config.yaml || true
	rm -f /tmp/$(IMAGE_NAME)-$(IMAGE_TAG).tar /tmp/$(CONTROLLER_IMAGE_NAME)-$(IMAGE_TAG).tar
	$(CONTAINER_ENGINE) save -o /tmp/$(IMAGE_NAME)-$(IMAGE_TAG).tar $(IMAGE_NAME):$(IMAGE_TAG)
	kind load image-archive /tmp/$(IMAGE_NAME)-$(IMAGE_TAG).tar --name $(KIND_CLUSTER_NAME)
	$(CONTAINER_ENGINE) save -o /tmp/$(CONTROLLER_IMAGE_NAME)-$(IMAGE_TAG).tar $(CONTROLLER_IMAGE_NAME):$(IMAGE_TAG)
	kind load image-archive /tmp/$(CONTROLLER_IMAGE_NAME)-$(IMAGE_TAG).tar --name $(KIND_CLUSTER_NAME)
	kubectl kustomize deploy/kind/ | kubectl apply -f -
	@echo "Waiting for PostgreSQL..."
	kubectl wait --for=condition=ready pod -l app=hypershell-postgres -n hypershell --timeout=120s
	@echo "Waiting for API server..."
	kubectl wait --for=condition=available deployment/hypershell-api-server -n hypershell --timeout=120s
	@echo "Waiting for controller..."
	kubectl wait --for=condition=available deployment/hypershell-controller -n hypershell --timeout=120s
	@echo ""
	@echo "HyperShell is running!"
	@echo "  API: http://localhost:$(KIND_API_PORT)/api/hypershell/v1/fleets"
	@echo "  API Server Logs: kubectl logs -f -l app=hypershell-api-server -n hypershell"
	@echo "  Controller Logs: kubectl logs -f -l app=hypershell-controller -n hypershell"

.PHONY: kind-down
kind-down:
	kind delete cluster --name $(KIND_CLUSTER_NAME)

.PHONY: kind-rebuild
kind-rebuild: images
	rm -f /tmp/$(IMAGE_NAME)-$(IMAGE_TAG).tar /tmp/$(CONTROLLER_IMAGE_NAME)-$(IMAGE_TAG).tar
	$(CONTAINER_ENGINE) save -o /tmp/$(IMAGE_NAME)-$(IMAGE_TAG).tar $(IMAGE_NAME):$(IMAGE_TAG)
	kind load image-archive /tmp/$(IMAGE_NAME)-$(IMAGE_TAG).tar --name $(KIND_CLUSTER_NAME)
	$(CONTAINER_ENGINE) save -o /tmp/$(CONTROLLER_IMAGE_NAME)-$(IMAGE_TAG).tar $(CONTROLLER_IMAGE_NAME):$(IMAGE_TAG)
	kind load image-archive /tmp/$(CONTROLLER_IMAGE_NAME)-$(IMAGE_TAG).tar --name $(KIND_CLUSTER_NAME)
	kubectl kustomize deploy/kind/ | kubectl apply -f -
	kubectl rollout restart deployment/hypershell-api-server -n hypershell
	kubectl rollout restart deployment/hypershell-controller -n hypershell
	kubectl wait --for=condition=available deployment/hypershell-api-server -n hypershell --timeout=120s
	kubectl wait --for=condition=available deployment/hypershell-controller -n hypershell --timeout=120s

.PHONY: kind-status
kind-status:
	@echo "=== Cluster ==="
	kubectl cluster-info --context kind-$(KIND_CLUSTER_NAME) 2>/dev/null || echo "Cluster not running"
	@echo ""
	@echo "=== Pods ==="
	kubectl get pods -n hypershell 2>/dev/null || echo "Namespace not found"
	@echo ""
	@echo "=== Services ==="
	kubectl get svc -n hypershell 2>/dev/null || echo "Namespace not found"
