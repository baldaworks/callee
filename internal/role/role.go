// Package role parses and validates Callee Markdown roles.
package role

import (
	"fmt"
	"strings"
)

// Metadata is the intentionally flat role frontmatter schema.
type Metadata struct {
	Description string            `yaml:"description"`
	Type        string            `yaml:"type"`
	Cmd         string            `yaml:"cmd,omitempty"`
	Model       string            `yaml:"model,omitempty"`
	Reasoning   string            `yaml:"reasoning,omitempty"`
	Mode        string            `yaml:"mode,omitempty"`
	ExtraArgs   []string          `yaml:"extra_args,omitempty"`
	Params      map[string]string `yaml:"params,omitempty"`
}

// Role is a validated Markdown-defined agent role.
type Role struct {
	ID       string
	Metadata Metadata
	Template string
}

var runtimeTypes = map[string]string{
	"codex":       "codex_acp",
	"claude":      "claude_code_acp",
	"opencode":    "opencode_acp",
	"copilot":     "copilot_acp",
	"grok":        "grok_acp",
	"generic_acp": "generic_acp",
}

// RuntimeType maps the public Callee type to its Norma Runtime type.
func RuntimeType(kind string) (string, bool) {
	v, ok := runtimeTypes[kind]

	return v, ok
}

// SupportedTypes returns the supported public types in stable order.
func SupportedTypes() []string {
	return []string{"codex", "claude", "opencode", "copilot", "grok", "generic_acp"}
}

// Validate validates role metadata and the prompt template.
func (r Role) Validate() error {
	if strings.TrimSpace(r.Metadata.Description) == "" {
		return fmt.Errorf("role %q: missing required frontmatter field \"description\"", r.ID)
	}

	if strings.TrimSpace(r.Metadata.Type) == "" {
		return fmt.Errorf("role %q: missing required frontmatter field \"type\"", r.ID)
	}

	if _, ok := RuntimeType(r.Metadata.Type); !ok {
		return fmt.Errorf("role %q: unsupported type %q", r.ID, r.Metadata.Type)
	}

	if r.Metadata.Type == "generic_acp" && strings.TrimSpace(r.Metadata.Cmd) == "" {
		return fmt.Errorf("role %q: type \"generic_acp\" requires a non-empty cmd", r.ID)
	}

	if strings.TrimSpace(r.Template) == "" {
		return fmt.Errorf("role %q: template body must not be empty", r.ID)
	}

	return validateTemplate(r.ID, r.Template, r.Metadata.Params)
}
