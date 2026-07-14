package role

import (
	"bytes"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// Parse parses one Markdown role file.
func Parse(id string, data []byte) (Role, error) {
	const delimiter = "---"
	lines := strings.Split(string(data), "\n")
	if len(lines) < 3 || strings.TrimSpace(lines[0]) != delimiter {
		return Role{}, fmt.Errorf("role %q: missing YAML frontmatter", id)
	}
	end := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == delimiter {
			end = i
			break
		}
	}
	if end < 0 {
		return Role{}, fmt.Errorf("role %q: missing YAML frontmatter closing delimiter", id)
	}
	var metadata Metadata
	decoder := yaml.NewDecoder(bytes.NewBufferString(strings.Join(lines[1:end], "\n")))
	decoder.KnownFields(true)
	if err := decoder.Decode(&metadata); err != nil {
		msg := err.Error()
		if i := strings.Index(msg, "field "); i >= 0 {
			msg = "unknown frontmatter " + msg[i:]
		}
		return Role{}, fmt.Errorf("role %q: %s", id, msg)
	}
	r := Role{ID: id, Metadata: metadata, Template: strings.Join(lines[end+1:], "\n")}
	if err := r.Validate(); err != nil {
		return Role{}, err
	}
	return r, nil
}
