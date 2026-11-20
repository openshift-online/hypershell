package cedar

import "testing"

// TestSchemaResolves confirms the embedded HyperShell Cedar schema parses and
// resolves (PLAT-PGP-03). A failure here is a defect in the embedded schema text.
func TestSchemaResolves(t *testing.T) {
	rs, err := resolvedSchema()
	if err != nil {
		t.Fatalf("resolvedSchema() error: %v", err)
	}
	if rs == nil {
		t.Fatal("resolvedSchema() returned nil schema")
	}
}

func TestSchemaVersionCarried(t *testing.T) {
	if SchemaVersion == "" {
		t.Fatal("SchemaVersion must be set")
	}
	if SchemaCedar() == "" {
		t.Fatal("SchemaCedar() must return the schema text")
	}
}
