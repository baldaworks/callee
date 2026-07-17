package agent

import (
	"os"
	"strings"
	"testing"
)

func TestWorkflowDependencyPins(t *testing.T) {
	t.Parallel()

	module, err := os.ReadFile("../../go.mod")
	if err != nil {
		t.Fatalf("os.ReadFile(../../go.mod): %v", err)
	}

	for _, dependency := range []string{
		"github.com/Masterminds/sprig/v3 v3.3.0",
		"github.com/normahq/runtime/v2 v2.0.7",
		"google.golang.org/adk/v2 v2.0.0",
	} {
		if !strings.Contains(string(module), dependency) {
			t.Errorf("go.mod does not pin %q", dependency)
		}
	}
}
