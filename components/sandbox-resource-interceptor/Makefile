.PHONY: proto
# Regenerate the Go stubs from the vendored OpenShell protos. Requires buf on
# PATH; the plugin versions are pinned in proto/buf.gen.yaml.
proto:
	cd proto && buf generate

.PHONY: proto-clean
proto-clean:
	rm -rf pkg/api/grpc/

.PHONY: test
test:
	go test -count=1 ./...
