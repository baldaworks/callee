package evaluation

import (
	"fmt"
	"strings"

	"github.com/baldaworks/callee/internal/agent"
)

// ValidateConfig checks trusted local service configuration without remote I/O.
func ValidateConfig(config Config, getenv func(string) string) error {
	if getenv == nil {
		return fmt.Errorf("environment lookup is required")
	}

	switch config.Kind {
	case agent.TypeSafeJevKind:
		if strings.TrimSpace(getenv("TYPESAFE_API_KEY")) == "" {
			return fmt.Errorf("environment variable TYPESAFE_API_KEY is required for TypeSafeJev")
		}

		model := strings.TrimSpace(config.Model)
		if model == "" {
			model = strings.TrimSpace(getenv("TYPESAFE_DEFAULT_MODEL"))
		}

		if model == "" {
			model = "jev-latest"
		}

		if !strings.HasPrefix(model, "jev-") {
			return fmt.Errorf("TypeSafe model must identify a Jev model")
		}

		_, err := resolveTypeSafeEndpoint(getenv("TYPESAFE_BASE_URL"))

		return err
	case agent.OpenRouterDecisionKind:
		if strings.TrimSpace(getenv("OPENROUTER_API_KEY")) == "" {
			return fmt.Errorf("environment variable OPENROUTER_API_KEY is required for OpenRouterDecision")
		}

		if strings.TrimSpace(config.Model) == "" {
			return fmt.Errorf("OpenRouterDecision requires nonblank model")
		}

		return nil
	default:
		return fmt.Errorf("unsupported evaluation kind %q", config.Kind)
	}
}
