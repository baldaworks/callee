// Package role parses and validates Callee Markdown roles.
package role

import (
	"fmt"
	"strings"
	"time"
)

const (
	// CurrentAPI is the API understood by this Callee release.
	CurrentAPI = "callee.metalagman.dev"
	// RoleKind identifies resources loaded from a roles directory.
	RoleKind = "role"
)

// Provider configures the ACP provider used by a role.
type Provider struct {
	Type      string   `yaml:"type"`
	Cmd       string   `yaml:"cmd,omitempty"`
	Model     string   `yaml:"model,omitempty"`
	Reasoning string   `yaml:"reasoning,omitempty"`
	Mode      string   `yaml:"mode,omitempty"`
	ExtraArgs []string `yaml:"extra_args,omitempty"`
	REPL      bool     `yaml:"repl,omitempty"`
	Timeout   string   `yaml:"timeout,omitempty"`
}

// Metadata is the role frontmatter schema.
type Metadata struct {
	API         string            `yaml:"api,omitempty"`
	Kind        string            `yaml:"kind,omitempty"`
	Description string            `yaml:"description"`
	Provider    Provider          `yaml:"provider"`
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
	if strings.TrimSpace(r.Metadata.API) == "" {
		return fmt.Errorf("role %q: missing api context", r.ID)
	}

	if api := r.API(); api != CurrentAPI {
		return fmt.Errorf("role %q: unsupported api %q", r.ID, api)
	}

	if strings.TrimSpace(r.Metadata.Kind) == "" {
		return fmt.Errorf("role %q: missing kind context", r.ID)
	}

	if kind := r.Kind(); kind != RoleKind {
		return fmt.Errorf("role %q: unsupported kind %q", r.ID, kind)
	}

	if strings.TrimSpace(r.Metadata.Description) == "" {
		return fmt.Errorf("role %q: missing required frontmatter field \"description\"", r.ID)
	}

	provider := r.Metadata.Provider
	if strings.TrimSpace(provider.Type) == "" {
		return fmt.Errorf("role %q: missing required frontmatter field \"provider.type\"", r.ID)
	}

	if _, ok := RuntimeType(provider.Type); !ok {
		return fmt.Errorf("role %q: unsupported provider.type %q", r.ID, provider.Type)
	}

	if provider.Type == "generic_acp" && strings.TrimSpace(provider.Cmd) == "" {
		return fmt.Errorf("role %q: provider.type \"generic_acp\" requires a non-empty provider.cmd", r.ID)
	}

	for index, arg := range provider.ExtraArgs {
		if strings.TrimSpace(arg) == "" {
			return fmt.Errorf("role %q: provider.extra_args[%d] must not be empty", r.ID, index)
		}
	}

	if provider.Timeout != "" {
		value, err := time.ParseDuration(provider.Timeout)
		if err != nil {
			return fmt.Errorf("role %q: provider.timeout %q is not a valid duration: %w", r.ID, provider.Timeout, err)
		}

		if value <= 0 {
			return fmt.Errorf("role %q: provider.timeout must be greater than zero", r.ID)
		}
	}

	if strings.TrimSpace(r.Template) == "" {
		return fmt.Errorf("role %q: template body must not be empty", r.ID)
	}

	return validateTemplate(r.ID, r.Template, r.Metadata.Params)
}

// API returns the role's effective API.
func (r Role) API() string {
	return strings.TrimSpace(r.Metadata.API)
}

// Kind returns the role's effective resource kind.
func (r Role) Kind() string {
	return strings.TrimSpace(r.Metadata.Kind)
}

// PromptTimeout returns the provider override or the supplied CLI default.
func (r Role) PromptTimeout(defaultValue time.Duration) time.Duration {
	if r.Metadata.Provider.Timeout == "" {
		return defaultValue
	}

	value, err := time.ParseDuration(r.Metadata.Provider.Timeout)
	if err != nil {
		return defaultValue
	}

	return value
}
