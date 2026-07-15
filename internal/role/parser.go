package role

import (
	"bytes"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// Defaults supplies resource metadata from the loader context.
type Defaults struct {
	API  string
	Kind string
}

// Parse parses one Markdown role file using defaults supplied by its loader.
func Parse(id string, data []byte, defaults Defaults) (Role, error) {
	frontmatter, template, err := splitMarkdownRole(id, data)
	if err != nil {
		return Role{}, err
	}

	metadata, err := parseMetadata(id, frontmatter, defaults)
	if err != nil {
		return Role{}, err
	}

	r := Role{ID: id, Metadata: metadata, Template: template}
	if err := r.Validate(); err != nil {
		return Role{}, err
	}

	return r, nil
}

func splitMarkdownRole(id string, data []byte) (string, string, error) {
	const delimiter = "---"

	lines := strings.Split(string(data), "\n")
	if len(lines) < 3 || strings.TrimSpace(lines[0]) != delimiter {
		return "", "", fmt.Errorf("role %q: missing YAML frontmatter", id)
	}

	end := -1

	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == delimiter {
			end = i

			break
		}
	}

	if end < 0 {
		return "", "", fmt.Errorf("role %q: missing YAML frontmatter closing delimiter", id)
	}

	return strings.Join(lines[1:end], "\n"), strings.Join(lines[end+1:], "\n"), nil
}

func parseMetadata(id, frontmatter string, defaults Defaults) (Metadata, error) {
	var metadata Metadata

	decoder := yaml.NewDecoder(bytes.NewBufferString(frontmatter))
	decoder.KnownFields(true)

	if err := decoder.Decode(&metadata); err != nil {
		msg := err.Error()
		if i := strings.Index(msg, "field "); i >= 0 {
			msg = "unknown frontmatter " + msg[i:]
		}

		return Metadata{}, fmt.Errorf("role %q: %s", id, msg)
	}

	var node yaml.Node

	if err := yaml.Unmarshal([]byte(frontmatter), &node); err != nil {
		return Metadata{}, fmt.Errorf("role %q: decode frontmatter presence: %w", id, err)
	}

	if err := validateOptionalString(id, "api", metadata.API, frontmatterValue(node, "api")); err != nil {
		return Metadata{}, err
	}

	if err := validateOptionalString(id, "kind", metadata.Kind, frontmatterValue(node, "kind")); err != nil {
		return Metadata{}, err
	}

	timeoutNode := frontmatterNestedValue(node, "provider", "timeout")
	if err := validateOptionalString(id, "provider.timeout", metadata.Provider.Timeout, timeoutNode); err != nil {
		return Metadata{}, err
	}

	metadata.API = defaultString(metadata.API, defaults.API)
	metadata.Kind = defaultString(metadata.Kind, defaults.Kind)

	return metadata, nil
}

func validateOptionalString(id, path, decoded string, node *yaml.Node) error {
	if node == nil {
		return nil
	}

	if strings.TrimSpace(decoded) == "" {
		return fmt.Errorf("role %q: frontmatter field %q must not be empty", id, path)
	}

	if node.Tag != "!!str" {
		return fmt.Errorf("role %q: frontmatter field %q must be a string", id, path)
	}

	return nil
}

func frontmatterValue(document yaml.Node, key string) *yaml.Node {
	if len(document.Content) == 0 {
		return nil
	}

	mapping := document.Content[0]
	for index := 0; index+1 < len(mapping.Content); index += 2 {
		if mapping.Content[index].Value == key {
			return mapping.Content[index+1]
		}
	}

	return nil
}

func frontmatterNestedValue(document yaml.Node, outer, inner string) *yaml.Node {
	if len(document.Content) == 0 {
		return nil
	}

	mapping := document.Content[0]
	for index := 0; index+1 < len(mapping.Content); index += 2 {
		if mapping.Content[index].Value == outer {
			nested := mapping.Content[index+1]
			for nestedIndex := 0; nestedIndex+1 < len(nested.Content); nestedIndex += 2 {
				if nested.Content[nestedIndex].Value == inner {
					return nested.Content[nestedIndex+1]
				}
			}
		}
	}

	return nil
}

func defaultString(value, defaultValue string) string {
	if strings.TrimSpace(value) == "" {
		return defaultValue
	}

	return strings.TrimSpace(value)
}
