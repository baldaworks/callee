package role

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// MarshalMarkdown serializes a validated role as a Markdown role file.
func (r Role) MarshalMarkdown() ([]byte, error) {
	if err := r.Validate(); err != nil {
		return nil, err
	}

	metadata, err := yaml.Marshal(r.Metadata)
	if err != nil {
		return nil, fmt.Errorf("marshal role %q metadata: %w", r.ID, err)
	}

	body := strings.TrimSuffix(r.Template, "\n")

	return []byte("---\n" + string(metadata) + "---\n\n" + body + "\n"), nil
}
