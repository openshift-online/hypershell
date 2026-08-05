package api

import (
	"embed"
	"io/fs"
)

//go:embed openapi/api/openapi.yaml
var openapiFS embed.FS

// GetOpenAPISpec returns the bundled OpenAPI document generated from the
// canonical split specifications in ../../openapi.
func GetOpenAPISpec() ([]byte, error) {
	return fs.ReadFile(openapiFS, "openapi/api/openapi.yaml")
}
