module github.com/openshift-online/hypershell/scripts/cli-generator

go 1.25.0

require github.com/openshift-online/hypershell/scripts/openapi-ir v0.0.0

require (
	github.com/getkin/kin-openapi v0.145.0 // indirect
	github.com/go-openapi/jsonpointer v1.0.0 // indirect
	github.com/kr/pretty v0.3.1 // indirect
	github.com/oasdiff/yaml v0.1.1 // indirect
	github.com/oasdiff/yaml3 v0.0.14 // indirect
	github.com/santhosh-tekuri/jsonschema/v6 v6.0.2 // indirect
	golang.org/x/text v0.14.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace github.com/openshift-online/hypershell/scripts/openapi-ir => ../openapi-ir
