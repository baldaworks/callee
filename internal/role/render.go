package role

import (
	"fmt"
	"regexp"
	"strings"
)

var expression = regexp.MustCompile(`\{\{([^}]*)\}\}`)

func validateTemplate(id, body string) error {
	matches := expression.FindAllStringSubmatch(body, -1)
	for _, match := range matches {
		if match[0] != "{{ prompt }}" {
			return fmt.Errorf("role %q: unsupported template expression %q", id, match[0])
		}
	}

	if len(matches) != 1 || strings.Count(body, "{{ prompt }}") != 1 {
		return fmt.Errorf("role %q: template must contain \"{{ prompt }}\" exactly once", id)
	}

	return nil
}

// Render substitutes the only supported template expression. Prompt text is not reinterpreted.
func (r Role) Render(prompt string) (string, error) {
	if err := validateTemplate(r.ID, r.Template); err != nil {
		return "", err
	}

	return strings.Replace(r.Template, "{{ prompt }}", prompt, 1), nil
}
