package agent

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

func TestEmbeddedSchemaMatchesCheckedInArtifact(t *testing.T) {
	t.Parallel()

	checkedIn, err := os.ReadFile("schema.json")
	if err != nil {
		t.Fatalf("os.ReadFile(schema.json): %v", err)
	}

	if !bytes.Equal(Schema(), checkedIn) {
		t.Fatal("embedded schema bytes differ from internal/agent/schema.json")
	}
}

func TestSchemaForKindReturnsStandaloneSchema(t *testing.T) {
	t.Parallel()

	tests := []struct {
		kind       Kind
		definition string
	}{
		{kind: RoleKind, definition: "role"},
		{kind: SequentialKind, definition: "sequential"},
		{kind: LoopKind, definition: "loop"},
	}

	for _, test := range tests {
		t.Run(string(test.kind), func(t *testing.T) {
			t.Parallel()

			data := requireSchemaForKind(t, test.kind)
			document := decodeSchemaDocument(t, test.kind, data)

			assertSchemaDocumentHeader(t, test.kind, test.definition, document)
			assertSchemaDefinitions(t, test.kind, test.definition, document)
			assertCompilableSchema(t, test.kind, data)
		})
	}
}

func TestSchemaForKindRejectsUnsupportedKind(t *testing.T) {
	t.Parallel()

	if _, err := SchemaForKind("Parallel"); err == nil {
		t.Fatal("SchemaForKind(Parallel) error = nil, want unsupported kind")
	}
}

func requireSchemaForKind(t *testing.T, kind Kind) []byte {
	t.Helper()

	data, err := SchemaForKind(kind)
	if err != nil {
		t.Fatalf("SchemaForKind(%q) error: %v", kind, err)
	}

	return data
}

func decodeSchemaDocument(t *testing.T, kind Kind, data []byte) map[string]any {
	t.Helper()

	var document map[string]any
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatalf("json.Unmarshal(%q): %v", kind, err)
	}

	return document
}

func assertSchemaDocumentHeader(t *testing.T, kind Kind, definition string, document map[string]any) {
	t.Helper()

	if document["$schema"] != "https://json-schema.org/draft/2020-12/schema" {
		t.Fatalf("schema[%q] $schema = %#v", kind, document["$schema"])
	}

	if _, exists := document["$id"]; exists {
		t.Fatalf("schema[%q] unexpectedly contains $id", kind)
	}

	if got, want := document["$ref"], "#/$defs/"+definition; got != want {
		t.Fatalf("schema[%q] $ref = %#v, want %q", kind, got, want)
	}
}

func assertSchemaDefinitions(t *testing.T, kind Kind, definition string, document map[string]any) {
	t.Helper()

	definitions, ok := document["$defs"].(map[string]any)
	if !ok {
		t.Fatalf("schema[%q] missing $defs object", kind)
	}

	if _, ok := definitions[definition]; !ok {
		t.Fatalf("schema[%q] missing selected definition %q", kind, definition)
	}

	for _, other := range []string{"role", "sequential", "loop"} {
		if other == definition {
			continue
		}

		if _, ok := definitions[other]; ok {
			t.Fatalf("schema[%q] unexpectedly contains unrelated definition %q", kind, other)
		}
	}
}

func assertCompilableSchema(t *testing.T, kind Kind, data []byte) {
	t.Helper()

	value, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("jsonschema.UnmarshalJSON(%q): %v", kind, err)
	}

	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)

	if err := compiler.AddResource("mem://callee-kind-schema.json", value); err != nil {
		t.Fatalf("compiler.AddResource(%q): %v", kind, err)
	}

	if _, err := compiler.Compile("mem://callee-kind-schema.json"); err != nil {
		t.Fatalf("compiler.Compile(%q): %v", kind, err)
	}
}
