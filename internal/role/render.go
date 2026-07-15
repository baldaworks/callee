package role

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

const expressionMatchGroups = 2

var (
	parameterName = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]*$`)
	expression    = regexp.MustCompile(`\{\{\s*([A-Za-z][A-Za-z0-9_-]*)\s*\}\}`)
)

func validateTemplate(id, body string, params map[string]string) error {
	if strings.Count(body, "{{ prompt }}") != 1 {
		return fmt.Errorf("role %q: template must contain \"{{ prompt }}\" exactly once", id)
	}

	occurrences := make(map[string]int)
	for _, match := range expression.FindAllStringSubmatch(body, -1) {
		occurrences[match[1]]++
	}

	for _, name := range sortedParameterNames(params) {
		if !parameterName.MatchString(name) {
			return fmt.Errorf("role %q: invalid parameter name %q", id, name)
		}

		if name == "prompt" {
			return fmt.Errorf("role %q: parameter name %q is reserved", id, name)
		}

		if strings.TrimSpace(params[name]) == "" {
			return fmt.Errorf("role %q: parameter %q requires a description", id, name)
		}

		if occurrences[name] == 0 {
			return fmt.Errorf("role %q: template must contain parameter expression \"{{ %s }}\" at least once", id, name)
		}
	}

	return nil
}

// Render substitutes the user prompt and every declared role parameter in one
// pass. Undeclared mustache fragments and mustache syntax in input values are
// preserved verbatim.
func (r Role) Render(prompt string, params map[string]string) (string, error) {
	if err := validateTemplate(r.ID, r.Template, r.Metadata.Params); err != nil {
		return "", err
	}

	missing := make([]string, 0)

	for name := range r.Metadata.Params {
		if _, ok := params[name]; !ok {
			missing = append(missing, name)
		}
	}

	unknown := make([]string, 0)

	for name := range params {
		if _, ok := r.Metadata.Params[name]; !ok {
			unknown = append(unknown, name)
		}
	}

	sort.Strings(missing)
	sort.Strings(unknown)

	if len(missing) > 0 || len(unknown) > 0 {
		return "", fmt.Errorf("role %q parameters: missing=%v unknown=%v", r.ID, missing, unknown)
	}

	return expression.ReplaceAllStringFunc(r.Template, func(fragment string) string {
		match := expression.FindStringSubmatch(fragment)
		if len(match) != expressionMatchGroups {
			return fragment
		}

		if match[1] == "prompt" && fragment == "{{ prompt }}" {
			return prompt
		}

		if value, ok := params[match[1]]; ok {
			return value
		}

		return fragment
	}), nil
}

func sortedParameterNames(params map[string]string) []string {
	names := make([]string, 0, len(params))
	for name := range params {
		names = append(names, name)
	}

	sort.Strings(names)

	return names
}
