// Package agent defines Callee's versioned Markdown and YAML agents.
package agent

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	// APIVersion is the resource API understood by this Callee release.
	APIVersion = "callee.metalagman.dev/v1alpha1"

	// RoleKind identifies a provider-backed leaf agent.
	RoleKind Kind = "Role"
	// SequentialKind identifies an ordered composite agent.
	SequentialKind Kind = "Sequential"
	// LoopKind identifies a bounded repeated composite agent.
	LoopKind Kind = "Loop"
)

const (
	defaultProviderTimeout = 15 * time.Minute
	defaultREPLTimeout     = 30 * time.Minute
)

var aliasPattern = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// Kind is a supported agent resource kind.
type Kind string

// Provider configures the ACP provider used by a Role.
type Provider struct {
	Type      string   `json:"type"                yaml:"type"`
	Cmd       string   `json:"cmd,omitempty"       yaml:"cmd,omitempty"`
	Model     string   `json:"model,omitempty"     yaml:"model,omitempty"`
	Reasoning string   `json:"reasoning,omitempty" yaml:"reasoning,omitempty"`
	Mode      string   `json:"mode,omitempty"      yaml:"mode,omitempty"`
	ExtraArgs []string `json:"extraArgs,omitempty" yaml:"extraArgs,omitempty"`
	Timeout   string   `json:"timeout,omitempty"   yaml:"timeout,omitempty"`
}

// Child identifies one ordered child occurrence in a composite.
type Child struct {
	Ref    string            `json:"ref"              yaml:"ref"`
	Alias  string            `json:"alias,omitempty"  yaml:"alias,omitempty"`
	Input  string            `json:"input,omitempty"  yaml:"input,omitempty"`
	State  map[string]any    `json:"state,omitempty"  yaml:"state,omitempty"`
	Params map[string]string `json:"params,omitempty" yaml:"params,omitempty"`
}

// Spec contains kind-specific authored configuration.
type Spec struct {
	Description   string            `json:"description"             yaml:"description"`
	Provider      *Provider         `json:"provider,omitempty"      yaml:"provider,omitempty"`
	REPL          *bool             `json:"repl,omitempty"          yaml:"repl,omitempty"`
	Params        map[string]string `json:"params,omitempty"        yaml:"params,omitempty"`
	State         map[string]any    `json:"state,omitempty"         yaml:"state,omitempty"`
	Children      []Child           `json:"children,omitempty"      yaml:"children,omitempty"`
	Body          string            `json:"body"                    yaml:"body,omitempty"`
	Output        string            `json:"output,omitempty"        yaml:"output,omitempty"`
	MaxIterations *int              `json:"maxIterations,omitempty" yaml:"maxIterations,omitempty"`
	OnExhausted   string            `json:"onExhausted,omitempty"   yaml:"onExhausted,omitempty"`
}

// Resource is one canonical agent resource. ID and Source are discovery data
// and are intentionally outside the versioned resource envelope.
type Resource struct {
	APIVersion string `json:"apiVersion" yaml:"apiVersion"`
	Kind       Kind   `json:"kind"       yaml:"kind"`
	Spec       Spec   `json:"spec"       yaml:"spec"`

	ID     string `json:"-" yaml:"-"`
	Source string `json:"-" yaml:"-"`
}

// RuntimeType maps public provider names to Norma Runtime names.
func RuntimeType(providerType string) (string, bool) {
	runtimeTypes := map[string]string{
		"codex":       "codex_acp",
		"claude":      "claude_code_acp",
		"opencode":    "opencode_acp",
		"copilot":     "copilot_acp",
		"grok":        "grok_acp",
		"cursor":      "generic_acp",
		"generic_acp": "generic_acp",
	}

	value, ok := runtimeTypes[providerType]

	return value, ok
}

// SupportedProviderTypes returns public provider names in stable order.
func SupportedProviderTypes() []string {
	return []string{"codex", "claude", "opencode", "copilot", "grok", "cursor", "generic_acp"}
}

// Validate checks semantic constraints not expressible in the JSON Schema.
func (r Resource) Validate() error {
	if err := validateSchema(r); err != nil {
		return fmt.Errorf("agent %q: validate schema: %w", r.ID, err)
	}

	if err := r.validateCommon(); err != nil {
		return err
	}

	if err := r.validateChildren(); err != nil {
		return err
	}

	return r.validateKind()
}

// ProviderTimeout returns the effective per-operation provider timeout.
func (r Resource) ProviderTimeout() time.Duration {
	if r.Spec.Provider == nil || r.Spec.Provider.Timeout == "" {
		return defaultProviderTimeout
	}

	value, err := time.ParseDuration(r.Spec.Provider.Timeout)
	if err != nil {
		return defaultProviderTimeout
	}

	return value
}

// REPL reports the effective Role REPL policy.
func (r Resource) REPL() bool {
	return r.Spec.REPL != nil && *r.Spec.REPL
}

// ExhaustionPolicy reports the effective Loop exhaustion policy.
func (r Resource) ExhaustionPolicy() string {
	if r.Spec.OnExhausted == "" {
		return "fail"
	}

	return r.Spec.OnExhausted
}

func (r Resource) validateCommon() error {
	if strings.TrimSpace(r.Spec.Description) == "" {
		return fmt.Errorf("agent %q: spec.description must not be blank", r.ID)
	}

	if strings.TrimSpace(r.Spec.Body) == "" {
		return fmt.Errorf("agent %q: spec.body must not be blank", r.ID)
	}

	if err := validateState(r.ID, "spec.state", r.Spec.State); err != nil {
		return err
	}

	if err := validateStateTemplates(r.ID+" spec.state", r.Spec.State); err != nil {
		return err
	}

	return nil
}

func (r Resource) validateChildren() error {
	for index, child := range r.Spec.Children {
		if strings.TrimSpace(child.Ref) == "" {
			return fmt.Errorf("agent %q: spec.children[%d].ref must not be blank", r.ID, index)
		}

		if child.Alias != "" && !aliasPattern.MatchString(child.Alias) {
			return fmt.Errorf("agent %q: spec.children[%d].alias %q must match %s", r.ID, index, child.Alias, aliasPattern)
		}

		if err := validateState(r.ID, fmt.Sprintf("spec.children[%d].state", index), child.State); err != nil {
			return err
		}

		if err := validateStateTemplates(fmt.Sprintf("%s spec.children[%d].state", r.ID, index), child.State); err != nil {
			return err
		}

		if child.Input != "" {
			if _, err := ParseTemplate(fmt.Sprintf("%s spec.children[%d].input", r.ID, index), child.Input); err != nil {
				return err
			}
		}

		for name, binding := range child.Params {
			if _, err := ParseRestrictedTemplate(fmt.Sprintf("%s spec.children[%d].params.%s", r.ID, index, name), binding); err != nil {
				return err
			}
		}
	}

	return nil
}

func (r Resource) validateKind() error {
	switch r.Kind {
	case RoleKind:
		return r.validateRole()
	case SequentialKind, LoopKind:
		if _, err := ParseTemplate(r.ID+" spec.body", r.Spec.Body); err != nil {
			return err
		}

		if r.Spec.Output != "" {
			if _, err := ParseOutputTemplate(r.ID+" spec.output", r.Spec.Output); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("agent %q: unsupported kind %q", r.ID, r.Kind)
	}

	return nil
}

func (r Resource) validateRole() error {
	provider := r.Spec.Provider
	if provider == nil {
		return fmt.Errorf("agent %q: Role requires spec.provider", r.ID)
	}

	if _, ok := RuntimeType(provider.Type); !ok {
		return fmt.Errorf("agent %q: unsupported spec.provider.type %q", r.ID, provider.Type)
	}

	if provider.Type == "generic_acp" && strings.TrimSpace(provider.Cmd) == "" {
		return fmt.Errorf("agent %q: spec.provider.type generic_acp requires nonblank spec.provider.cmd", r.ID)
	}

	for index, arg := range provider.ExtraArgs {
		if strings.TrimSpace(arg) == "" {
			return fmt.Errorf("agent %q: spec.provider.extraArgs[%d] must not be blank", r.ID, index)
		}
	}

	if provider.Timeout != "" {
		timeout, err := time.ParseDuration(provider.Timeout)
		if err != nil {
			return fmt.Errorf("agent %q: spec.provider.timeout %q: %w", r.ID, provider.Timeout, err)
		}

		if timeout <= 0 {
			return fmt.Errorf("agent %q: spec.provider.timeout must be greater than zero", r.ID)
		}
	}

	for name, description := range r.Spec.Params {
		if !parameterName.MatchString(name) {
			return fmt.Errorf("agent %q: invalid parameter name %q", r.ID, name)
		}

		if strings.TrimSpace(description) == "" {
			return fmt.Errorf("agent %q: parameter %q requires a description", r.ID, name)
		}
	}

	if err := ValidateRoleTemplateMigration(r.ID, r.Spec.Body, r.Spec.Params); err != nil {
		return err
	}

	return ValidateRoleTemplate(r.ID, r.Spec.Body)
}

// DefaultREPLTimeout returns the CLI operator-wait default.
func DefaultREPLTimeout() time.Duration {
	return defaultREPLTimeout
}

// UnmarshalYAML accepts either a scalar resource ID or a child mapping.
func (c *Child) UnmarshalYAML(node *yaml.Node) error {
	if node.ShortTag() == "!!str" {
		var ref string
		if err := node.Decode(&ref); err != nil {
			return err
		}

		*c = Child{Ref: ref}

		return nil
	}

	if node.Kind != yaml.MappingNode {
		return fmt.Errorf("child must be a resource ID or mapping")
	}

	allowed := map[string]bool{"ref": true, "alias": true, "input": true, "state": true, "params": true}
	seen := make(map[string]bool)

	for index := 0; index+1 < len(node.Content); index += 2 {
		name := node.Content[index].Value
		if !allowed[name] {
			return fmt.Errorf("unknown child field %q", name)
		}

		if seen[name] {
			return fmt.Errorf("duplicate child field %q", name)
		}

		seen[name] = true
	}

	type child Child

	var value child
	if err := node.Decode(&value); err != nil {
		return err
	}

	*c = Child(value)

	return nil
}
