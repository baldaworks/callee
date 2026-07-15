package cli

import (
	"fmt"
	"os"
	"strings"
)

func parameterValues(raw, files []string, label, duplicate string) (map[string]string, error) {
	values := make(map[string]string, len(raw)+len(files))
	for _, assignment := range raw {
		name, value, err := parseParameterAssignment(label, assignment)
		if err != nil {
			return nil, err
		}

		if _, exists := values[name]; exists {
			return nil, fmt.Errorf("%s %q %s", label, name, duplicate)
		}

		values[name] = value
	}

	for _, assignment := range files {
		name, path, err := parseParameterAssignment(label, assignment)
		if err != nil {
			return nil, err
		}

		if _, exists := values[name]; exists {
			return nil, fmt.Errorf("%s %q %s", label, name, duplicate)
		}

		if path == "" {
			return nil, fmt.Errorf("%s %q requires a non-empty file path", label, name)
		}

		if path == "-" {
			return nil, fmt.Errorf("%s %q requires a file path, not stdin", label, name)
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read %s %q from %q: %w", label, name, path, err)
		}

		values[name] = string(content)
	}

	return values, nil
}

func parseParameterAssignment(label, assignment string) (string, string, error) {
	name, value, ok := strings.Cut(assignment, "=")

	name = strings.TrimSpace(name)
	if !ok || name == "" {
		return "", "", fmt.Errorf("%s %q must use key=value", label, assignment)
	}

	return name, value, nil
}
