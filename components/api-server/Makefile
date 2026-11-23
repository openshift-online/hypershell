BINARY_NAME=hypershell
CONTAINER_ENGINE?=$(shell command -v podman 2>/dev/null || echo docker)

# On macOS, testcontainers-go cannot discover the Podman socket automatically.
# Export DOCKER_HOST and disable the Ryuk reaper so testcontainers works with
# Podman Machine without requiring the user to set these manually.
ifeq ($(shell uname -s),Darwin)
  ifndef DOCKER_HOST
    _PODMAN_SOCK:=$(shell podman machine inspect --format '{{.ConnectionInfo.PodmanSocket.Path}}' 2>/dev/null)
    ifneq ($(_PODMAN_SOCK),)
      export DOCKER_HOST:=unix://$(_PODMAN_SOCK)
      export TESTCONTAINERS_RYUK_DISABLED:=true
    endif
  endif
endif
POSTGRES_IMAGE=postgres:18@sha256:a02db8cac496f15b094798a38254f14d6e00741f709360e5e00bb6668ea31636
OPENAPI_SPEC=$(abspath openapi/openapi.yaml)

# OpenAPI generation path selection (agent-runtime-images.spec.md, ARI-06).
# The default `container` path builds Dockerfile.openapi with a container engine.
# The `jar` path runs the same openapi-generator-cli version with identical
# arguments and post-processing and needs no container engine; it is the path the
# container-free agent-runtime sandbox uses. Both paths MUST produce byte-identical
# output (asserted in CI).
OPENAPI_GENERATOR?=container
OPENAPI_GENERATOR_JAR?=/usr/local/lib/openapi-generator-cli/openapi-generator-cli.jar
SDK_GENERATOR_DIR=$(abspath ../../scripts/sdk-generator)
SDK_TYPESCRIPT_DIR=$(abspath ../sdk-typescript)
SDK_GO_DIR=$(abspath ../sdk-go)
SDK_API_PREFIX=/api/hypershell
SDK_GO_MODULE=github.com/openshift-online/hypershell/components/sdk-go

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
	API_ENV=integration_testing go test -p 1 -v ./... -count=1 $(GO_TEST_FLAGS)

.PHONY: test-integration
test-integration: install
	API_ENV=integration_testing go test -p 1 -v ./plugins/... -count=1

# Container-free integration tests (agent-runtime-images.spec.md, ARI-06). Starts
# a throwaway PostgreSQL with initdb/pg_ctl in a temporary directory, runs the
# integration suite with DB_FACTORY_MODE=external against the committed secrets/db.*
# settings, and stops and removes the database on every exit. This is the path the
# container-free agent-runtime sandbox uses in place of the testcontainers default.
.PHONY: test-integration-external-db
test-integration-external-db: install
	bash hack/integration-external-db.sh

.PHONY: clean
clean:
	rm -f $(BINARY_NAME)

.PHONY: generate
generate:
	rm -rf pkg/api/openapi
ifeq ($(OPENAPI_GENERATOR),jar)
	# Container-free path (ARI-06): same generator version, arguments, and
	# post-processing as Dockerfile.openapi (generate -g go, then drop
	# go.mod/go.sum/test and gofmt). Run from the component root with relative
	# paths, exactly as the container does under /local, so output matches
	# byte-for-byte.
	java -jar "$(OPENAPI_GENERATOR_JAR)" generate -i openapi/openapi.yaml -g go -o pkg/api/openapi
	rm -f pkg/api/openapi/go.mod pkg/api/openapi/go.sum
	rm -rf pkg/api/openapi/test
	gofmt -w pkg/api/openapi
else
	$(CONTAINER_ENGINE) build -t $(BINARY_NAME)-openapi -f Dockerfile.openapi .
	id=$$($(CONTAINER_ENGINE) create $(BINARY_NAME)-openapi) && \
	$(CONTAINER_ENGINE) cp $$id:/local/pkg/api/openapi ./pkg/api/openapi && \
	$(CONTAINER_ENGINE) rm $$id
endif
	# The upstream template emits trailing whitespace and extra EOF newlines in
	# generated Markdown. Normalize every generated document so repository
	# whitespace checks and the drift job reproduce committed files byte-for-byte.
	find pkg/api/openapi/docs -type f -name '*.md' -exec perl -0777 -i -pe 's/[ \t]+$$//mg; s/\n+\z/\n/' {} +

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

# The CLI is generated by the rh-trex-ai cli-generator pinned in
# scripts/rh-trex-ai.ref; see scripts/generate-cli.sh.
generate-cli:
	$(MAKE) -C ../.. generate-cli

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
