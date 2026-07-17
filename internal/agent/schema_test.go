package agent

import (
	"bytes"
	"os"
	"testing"
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
