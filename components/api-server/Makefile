BINARY_NAME=hypershell
CONTAINER_ENGINE?=$(shell command -v podman 2>/dev/null || echo docker)
POSTGRES_IMAGE=postgres:18@sha256:a02db8cac496f15b094798a38254f14d6e00741f709360e5e00bb6668ea31636
OPENAPI_SPEC=$(abspath openapi/openapi.yaml)
SDK_GENERATOR_DIR=$(abspath ../../scripts/sdk-generator)
CLI_GENERATOR_DIR=$(abspath ../../scripts/cli-generator)
SDK_TYPESCRIPT_DIR=$(abspath ../sdk-typescript)
SDK_GO_DIR=$(abspath ../sdk-go)
CLI_DIR=$(abspath ../cli)
SDK_API_PREFIX=/api/hypershell/v1
SDK_GO_MODULE=github.com/openshift-online/hypershell/components/sdk-go
CLI_MODULE=github.com/openshift-online/hypershell/components/cli

# Version information for ldflags
git_sha:=$(shell git rev-parse HEAD 2>/dev/null || echo "unknown")
build_version:=$(shell HYPERSHELL_VCS_REF=$(git_sha) ../../scripts/build-version.sh local 2>/dev/null || echo "dev-unknown")
build_time:=$(shell date -u '+%Y-%m-%d %H:%M:%S UTC')
ldflags=-X github.com/openshift-online/rh-trex-ai/pkg/api.Version=$(build_version) -X 'github.com/openshift-online/rh-trex-ai/pkg/api.BuildTime=$(build_time)'

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

.PHONY: generate-sdk generate-sdk-ts generate-sdk-typescript generate-sdk-go generate-cli
generate-sdk: generate-sdk-ts generate-sdk-go
generate-sdk-typescript: generate-sdk-ts

generate-sdk-go:
	test -f "$(SDK_GENERATOR_DIR)/go.mod"
	rm -rf "$(SDK_GO_DIR)/types" "$(SDK_GO_DIR)/client"
	cd "$(SDK_GENERATOR_DIR)" && GOWORK=off go run . \
		--spec "$(OPENAPI_SPEC)" \
		--go-out "$(SDK_GO_DIR)" \
		--api-prefix "$(SDK_API_PREFIX)" \
		--project hypershell \
		--module "$(SDK_GO_MODULE)"
	find "$(SDK_GO_DIR)" -type f -name '*.go' -exec perl -i -p \
		-e 's|^// Source: .*|// Source: components/api-server/openapi/openapi.yaml|;' \
		-e 's|^// Generated:.*\n?||;' {} +
	gofmt -w "$(SDK_GO_DIR)"

generate-cli:
	test -f "$(CLI_GENERATOR_DIR)/go.mod"
	# Preserve the reviewed apply/delete commands; generated files are replaced
	# in place and scoped-resource commands are added alongside them.
	cd "$(CLI_GENERATOR_DIR)" && GOWORK=off go run . \
		--spec "$(OPENAPI_SPEC)" \
		--out "$(CLI_DIR)" \
		--binary hsctl \
		--project hypershell \
		--api-prefix "$(SDK_API_PREFIX)" \
		--module "$(CLI_MODULE)"

generate-sdk-ts:
	test -f "$(SDK_GENERATOR_DIR)/go.mod"
	rm -rf "$(SDK_TYPESCRIPT_DIR)/src"
	cd "$(SDK_GENERATOR_DIR)" && GOWORK=off go run . \
		--spec "$(OPENAPI_SPEC)" \
		--ts-out "$(SDK_TYPESCRIPT_DIR)" \
		--api-prefix "$(SDK_API_PREFIX)" \
		--project hypershell
	find "$(SDK_TYPESCRIPT_DIR)/src" -type f -name '*.ts' -exec perl -i -p \
		-e 's|^// Source: .*|// Source: components/api-server/openapi/openapi.yaml|;' \
		-e 's|^// Generated:.*\n?||;' {} +
	find "$(SDK_TYPESCRIPT_DIR)/src" -type f -name '*.ts' -exec perl -0777 -i -pe 's/\n+\z/\n/' {} +

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
