// Package agent defines Callee's versioned Markdown and YAML agents.
package agent

import (
	"encoding/json"
	"fmt"
	"math"
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
	// ScriptKind identifies a deterministic local validator leaf.
	ScriptKind Kind = "Script"
	// HumanKind identifies an operator-backed interactive leaf.
	HumanKind Kind = "Human"
	// JevKind identifies a remote typed-judgment leaf.
	JevKind Kind = "Jev"
	// SequentialKind identifies an ordered composite agent.
	SequentialKind Kind = "Sequential"
	// LoopKind identifies a bounded repeated composite agent.
	LoopKind Kind = "Loop"
	// RouterKind identifies a deterministic routed composite agent.
	RouterKind Kind = "Router"
)

const (
	// PermissionModeAsk prompts the operator for every ACP permission request.
	PermissionModeAsk PermissionMode = "ask"
	// PermissionModeAllow automatically selects a compatible allow option.
	PermissionModeAllow PermissionMode = "allow"
	// PermissionModeDeny automatically selects a compatible reject option.
	PermissionModeDeny PermissionMode = "deny"
)

const (
	defaultProviderTimeout = 15 * time.Minute
	defaultREPLTimeout     = 30 * time.Minute
	defaultScriptShell     = "sh"
	defaultJevTimeout      = 30 * time.Second
)

var aliasPattern = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// Kind is a supported agent resource kind.
type Kind string

// PermissionMode controls how a Role handles ACP permission requests.
type PermissionMode string

// Permissions configures Role-level ACP permission handling.
type Permissions struct {
	Mode PermissionMode `json:"mode" yaml:"mode"`
}

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

// JevAPI selects the fixed API transport used by a Jev resource.
type JevAPI struct {
	Type    string `json:"type"              yaml:"type"`
	Model   string `json:"model,omitempty"   yaml:"model,omitempty"`
	Timeout string `json:"timeout,omitempty" yaml:"timeout,omitempty"`
}

// JevQuestion defines one typed judgment in a batched Jev evaluation.
type JevQuestion struct {
	Type         string `json:"type"               yaml:"type"`
	Instructions any    `json:"instructions"       yaml:"instructions"`
	Criteria     any    `json:"criteria,omitempty" yaml:"criteria,omitempty"`
}

// Child identifies one ordered child occurrence in a composite.
type Child struct {
	Ref         string            `json:"ref"                   yaml:"ref"`
	Alias       string            `json:"alias,omitempty"       yaml:"alias,omitempty"`
	CanEscalate bool              `json:"canEscalate,omitempty" yaml:"canEscalate,omitempty"`
	Route       string            `json:"route,omitempty"       yaml:"route,omitempty"`
	Default     bool              `json:"default,omitempty"     yaml:"default,omitempty"`
	Input       string            `json:"input,omitempty"       yaml:"input,omitempty"`
	State       map[string]any    `json:"state,omitempty"       yaml:"state,omitempty"`
	Params      map[string]string `json:"params,omitempty"      yaml:"params,omitempty"`
}

// Spec contains kind-specific authored configuration.
type Spec struct {
	Description   string                 `json:"description"             yaml:"description"`
	Provider      *Provider              `json:"provider,omitempty"      yaml:"provider,omitempty"`
	API           *JevAPI                `json:"api,omitempty"           yaml:"api,omitempty"`
	Evidence      any                    `json:"evidence,omitempty"      yaml:"evidence,omitempty"`
	Questions     map[string]JevQuestion `json:"questions,omitempty"     yaml:"questions,omitempty"`
	Permissions   *Permissions           `json:"permissions,omitempty"   yaml:"permissions,omitempty"`
	Interactive   *bool                  `json:"interactive,omitempty"   yaml:"interactive,omitempty"`
	LegacyREPL    *bool                  `json:"repl,omitempty"          yaml:"repl,omitempty"`
	Params        map[string]string      `json:"params,omitempty"        yaml:"params,omitempty"`
	ResponseKey   string                 `json:"responseKey,omitempty"   yaml:"responseKey,omitempty"`
	Shell         string                 `json:"shell,omitempty"         yaml:"shell,omitempty"`
	Cwd           string                 `json:"cwd,omitempty"           yaml:"cwd,omitempty"`
	Env           map[string]string      `json:"env,omitempty"           yaml:"env,omitempty"`
	Timeout       string                 `json:"timeout,omitempty"       yaml:"timeout,omitempty"`
	OnNonZero     string                 `json:"onNonZero,omitempty"     yaml:"onNonZero,omitempty"`
	State         map[string]any         `json:"state,omitempty"         yaml:"state,omitempty"`
	Children      []Child                `json:"children,omitempty"      yaml:"children,omitempty"`
	Route         string                 `json:"route,omitempty"         yaml:"route,omitempty"`
	Body          string                 `json:"body"                    yaml:"body,omitempty"`
	Output        string                 `json:"output,omitempty"        yaml:"output,omitempty"`
	MaxIterations *int                   `json:"maxIterations,omitempty" yaml:"maxIterations,omitempty"`
	OnExhausted   string                 `json:"onExhausted,omitempty"   yaml:"onExhausted,omitempty"`
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
	if err := r.validateInteractiveCompat(); err != nil {
		return err
	}

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

// Interactive reports the effective Role interactive multi-turn policy.
func (r Resource) Interactive() bool {
	return r.Spec.InteractiveValue()
}

// REPL reports the effective Role REPL policy.
func (r Resource) REPL() bool {
	return r.Interactive()
}

// ScriptShell returns the effective shell used by a Script.
func (r Resource) ScriptShell() string {
	if strings.TrimSpace(r.Spec.Shell) == "" {
		return defaultScriptShell
	}

	return strings.TrimSpace(r.Spec.Shell)
}

// ScriptTimeout returns the effective per-script execution timeout.
func (r Resource) ScriptTimeout() time.Duration {
	if strings.TrimSpace(r.Spec.Timeout) == "" {
		return defaultProviderTimeout
	}

	value, err := time.ParseDuration(r.Spec.Timeout)
	if err != nil {
		return defaultProviderTimeout
	}

	return value
}

// JevTimeout returns the total budget for one Jev visit.
func (r Resource) JevTimeout() time.Duration {
	if r.Spec.API == nil || strings.TrimSpace(r.Spec.API.Timeout) == "" {
		return defaultJevTimeout
	}

	value, err := time.ParseDuration(r.Spec.API.Timeout)
	if err != nil {
		return defaultJevTimeout
	}

	return value
}

// JevModel returns the selected model or the transport-specific stable alias.
func (r Resource) JevModel() string {
	if r.Spec.API == nil {
		return ""
	}

	if model := strings.TrimSpace(r.Spec.API.Model); model != "" {
		return model
	}

	if r.Spec.API.Type == "openrouter" {
		return "~typesafe/jev-latest"
	}

	return "jev-latest"
}

// NonZeroPolicy reports the effective Script exit handling policy.
func (r Resource) NonZeroPolicy() string {
	if r.Spec.OnNonZero == "" {
		return "fail"
	}

	return r.Spec.OnNonZero
}

// EffectivePermissionMode reports the Role permission mode, defaulting to ask.
func (r Resource) EffectivePermissionMode() PermissionMode {
	if r.Spec.Permissions == nil || r.Spec.Permissions.Mode == "" {
		return PermissionModeAsk
	}

	return r.Spec.Permissions.Mode
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

	if r.Kind != JevKind && strings.TrimSpace(r.Spec.Body) == "" {
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

func (r Resource) validateInteractiveCompat() error {
	if _, err := r.Spec.canonicalInteractivePointer(); err != nil {
		return fmt.Errorf("agent %q: %w", r.ID, err)
	}

	return nil
}

func (r Resource) validateChildren() error {
	namedRoutes := make(map[string]int)
	defaultIndex := -1

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

		if err := r.validateChildRouting(index, child, namedRoutes, &defaultIndex); err != nil {
			return err
		}
	}

	return nil
}

func (r Resource) validateChildRouting(index int, child Child, namedRoutes map[string]int, defaultIndex *int) error {
	if r.Kind != RouterKind {
		if child.Route != "" || child.Default {
			return fmt.Errorf("agent %q: spec.children[%d].route and default are valid only for Router", r.ID, index)
		}

		return nil
	}

	route := strings.TrimSpace(child.Route)
	switch {
	case route == "" && !child.Default:
		return fmt.Errorf("agent %q: spec.children[%d] must declare exactly one of route or default=true", r.ID, index)
	case route != "" && child.Default:
		return fmt.Errorf("agent %q: spec.children[%d] must not declare both route and default=true", r.ID, index)
	case child.Route != route:
		return fmt.Errorf("agent %q: spec.children[%d].route %q must not have leading or trailing whitespace", r.ID, index, child.Route)
	case child.Default && *defaultIndex >= 0:
		return fmt.Errorf("agent %q: spec.children[%d].default duplicates spec.children[%d].default", r.ID, index, *defaultIndex)
	case child.Default:
		*defaultIndex = index
	default:
		if previous, exists := namedRoutes[route]; exists {
			return fmt.Errorf("agent %q: spec.children[%d].route %q duplicates spec.children[%d].route", r.ID, index, route, previous)
		}

		namedRoutes[route] = index
	}

	return nil
}

func (r Resource) validateKind() error {
	switch r.Kind {
	case RoleKind:
		return r.validateRole()
	case ScriptKind:
		return r.validateScript()
	case HumanKind:
		return r.validateHuman()
	case JevKind:
		return r.validateJev()
	case SequentialKind, LoopKind, RouterKind:
		if _, err := ParseTemplate(r.ID+" spec.body", r.Spec.Body); err != nil {
			return err
		}

		if r.Spec.Output != "" {
			if _, err := ParseOutputTemplate(r.ID+" spec.output", r.Spec.Output); err != nil {
				return err
			}
		}

		if r.Kind == RouterKind {
			if strings.TrimSpace(r.Spec.Route) == "" {
				return fmt.Errorf("agent %q: Router requires nonblank spec.route", r.ID)
			}

			if _, err := ParseTemplate(r.ID+" spec.route", r.Spec.Route); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("agent %q: unsupported kind %q", r.ID, r.Kind)
	}

	return nil
}

func (r Resource) validateJev() error {
	hasBody := strings.TrimSpace(r.Spec.Body) != ""
	hasEvidence := r.Spec.Evidence != nil

	if hasBody == hasEvidence {
		return fmt.Errorf("agent %q: Jev requires exactly one of spec.body or spec.evidence", r.ID)
	}

	if r.Spec.API == nil {
		return fmt.Errorf("agent %q: Jev requires spec.api", r.ID)
	}

	if err := r.validateJevAPI(); err != nil {
		return err
	}

	if hasBody {
		if _, err := ParseRestrictedTemplate(r.ID+" spec.body", r.Spec.Body); err != nil {
			return err
		}
	} else if err := validateJevContent(r.ID+" spec.evidence", r.Spec.Evidence, false); err != nil {
		return err
	}

	return r.validateJevQuestions()
}

func (r Resource) validateJevAPI() error {
	switch r.Spec.API.Type {
	case "typesafe":
		if model := strings.TrimSpace(r.Spec.API.Model); model != "" && !strings.HasPrefix(model, "jev-") {
			return fmt.Errorf("agent %q: spec.api.model %q must identify a TypeSafe Jev model", r.ID, r.Spec.API.Model)
		}
	case "openrouter":
		if model := strings.TrimSpace(r.Spec.API.Model); model != "" &&
			!strings.HasPrefix(model, "typesafe/jev-") && !strings.HasPrefix(model, "~typesafe/jev-") {
			return fmt.Errorf("agent %q: spec.api.model %q must identify an OpenRouter TypeSafe Jev model", r.ID, r.Spec.API.Model)
		}
	default:
		return fmt.Errorf("agent %q: spec.api.type %q must be typesafe or openrouter", r.ID, r.Spec.API.Type)
	}

	if r.Spec.API.Model != strings.TrimSpace(r.Spec.API.Model) {
		return fmt.Errorf("agent %q: spec.api.model must not have leading or trailing whitespace", r.ID)
	}

	if r.Spec.API.Timeout != "" {
		timeout, err := time.ParseDuration(r.Spec.API.Timeout)
		if err != nil {
			return fmt.Errorf("agent %q: spec.api.timeout %q: %w", r.ID, r.Spec.API.Timeout, err)
		}

		if timeout <= 0 {
			return fmt.Errorf("agent %q: spec.api.timeout must be greater than zero", r.ID)
		}
	}

	return nil
}

func (r Resource) validateJevQuestions() error {
	if len(r.Spec.Questions) == 0 {
		return fmt.Errorf("agent %q: Jev requires at least one spec.questions entry", r.ID)
	}

	for id, question := range r.Spec.Questions {
		if strings.TrimSpace(id) == "" || id != strings.TrimSpace(id) {
			return fmt.Errorf("agent %q: spec.questions contains invalid ID %q", r.ID, id)
		}

		path := fmt.Sprintf("%s spec.questions.%s", r.ID, id)
		if err := validateJevContent(path+".instructions", question.Instructions, false); err != nil {
			return err
		}

		if err := validateJevQuestion(path, r.Spec.API.Type, question); err != nil {
			return err
		}
	}

	return nil
}

func validateJevQuestion(path, apiType string, question JevQuestion) error {
	switch question.Type {
	case "choice":
		return validateJevChoiceQuestion(path, question.Criteria)
	case "score":
		return validateJevScoreQuestion(path, question.Criteria)
	case "noul":
		return validateJevNoulQuestion(path, apiType, question.Criteria)
	default:
		return fmt.Errorf("%s.type %q must be choice, noul, or score", path, question.Type)
	}
}

func validateJevChoiceQuestion(path string, value any) error {
	criteria, ok := value.(map[string]any)
	if !ok || len(criteria) < 2 || len(criteria) > 255 {
		return fmt.Errorf("%s.criteria must be an object with 2 to 255 choices", path)
	}

	for name, description := range criteria {
		if strings.TrimSpace(name) == "" || name != strings.TrimSpace(name) {
			return fmt.Errorf("%s.criteria contains invalid choice %q", path, name)
		}

		if description != nil {
			if err := validateJevContent(path+".criteria."+name, description, false); err != nil {
				return err
			}
		}
	}

	return nil
}

func validateJevScoreQuestion(path string, value any) error {
	criteria, ok := value.([]any)
	if !ok || len(criteria) < 2 || len(criteria) > 10 {
		return fmt.Errorf("%s.criteria must be an array with 2 to 10 levels", path)
	}

	for index, description := range criteria {
		if description == nil {
			continue
		}

		if err := validateJevContent(fmt.Sprintf("%s.criteria[%d]", path, index), description, false); err != nil {
			return err
		}
	}

	return nil
}

func validateJevNoulQuestion(path, apiType string, value any) error {
	if value == nil {
		return nil
	}

	criteria, ok := value.(map[string]any)
	if !ok || len(criteria) == 0 || len(criteria) > 2 {
		return fmt.Errorf("%s.criteria must describe true, false, or both", path)
	}

	for name, description := range criteria {
		if name != "true" && name != "false" {
			return fmt.Errorf("%s.criteria contains unsupported outcome %q", path, name)
		}

		if description != nil {
			if err := validateJevContent(path+".criteria."+name, description, false); err != nil {
				return err
			}
		}
	}

	if apiType == "openrouter" && len(criteria) != 2 {
		return fmt.Errorf("%s.criteria must describe both true and false for openrouter", path)
	}

	return nil
}

func validateJevContent(path string, value any, nested bool) error {
	switch typed := value.(type) {
	case nil:
		if nested {
			return nil
		}

		return fmt.Errorf("%s must be a string, object, or array", path)
	case string:
		_, err := ParseRestrictedTemplate(path, typed)

		return err
	case bool, json.Number, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		if nested {
			return nil
		}

		return fmt.Errorf("%s must be a string, object, or array", path)
	case float32:
		return validateJevNumber(path, float64(typed), nested)
	case float64:
		return validateJevNumber(path, typed, nested)
	case []any:
		for index, item := range typed {
			if err := validateJevContent(fmt.Sprintf("%s[%d]", path, index), item, true); err != nil {
				return err
			}
		}
	case map[string]any:
		for key, item := range typed {
			if err := validateJevContent(path+"."+key, item, true); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("%s contains unsupported value of type %T", path, value)
	}

	return nil
}

func validateJevNumber(path string, value float64, nested bool) error {
	if !nested {
		return fmt.Errorf("%s must be a string, object, or array", path)
	}

	if math.IsInf(value, 0) || math.IsNaN(value) {
		return fmt.Errorf("%s numbers must be finite", path)
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

	switch r.EffectivePermissionMode() {
	case PermissionModeAsk, PermissionModeAllow, PermissionModeDeny:
	default:
		return fmt.Errorf("agent %q: spec.permissions.mode %q must be ask, allow, or deny", r.ID, r.Spec.Permissions.Mode)
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

func (r Resource) validateScript() error {
	if _, err := ParseRestrictedTemplate(r.ID+" spec.body", r.Spec.Body); err != nil {
		return err
	}

	switch r.ScriptShell() {
	case "sh", "bash":
	default:
		return fmt.Errorf("agent %q: spec.shell %q must be sh or bash", r.ID, r.Spec.Shell)
	}

	if r.Spec.Cwd != "" {
		if _, err := ParseRestrictedTemplate(r.ID+" spec.cwd", r.Spec.Cwd); err != nil {
			return err
		}
	}

	for name, value := range r.Spec.Env {
		if strings.TrimSpace(name) == "" {
			return fmt.Errorf("agent %q: spec.env contains a blank name", r.ID)
		}

		if strings.Contains(name, "=") {
			return fmt.Errorf("agent %q: spec.env name %q must not contain =", r.ID, name)
		}

		if _, err := ParseRestrictedTemplate(r.ID+" spec.env."+name, value); err != nil {
			return err
		}
	}

	if r.Spec.Timeout != "" {
		timeout, err := time.ParseDuration(r.Spec.Timeout)
		if err != nil {
			return fmt.Errorf("agent %q: spec.timeout %q: %w", r.ID, r.Spec.Timeout, err)
		}

		if timeout <= 0 {
			return fmt.Errorf("agent %q: spec.timeout must be greater than zero", r.ID)
		}
	}

	switch r.NonZeroPolicy() {
	case "fail", "continue":
	default:
		return fmt.Errorf("agent %q: spec.onNonZero %q must be fail or continue", r.ID, r.Spec.OnNonZero)
	}

	return nil
}

func (r Resource) validateHuman() error {
	if strings.TrimSpace(r.Spec.ResponseKey) == "" {
		return fmt.Errorf("agent %q: Human requires nonblank spec.responseKey", r.ID)
	}

	switch r.Spec.ResponseKey {
	case "outputs", "scripts", "evaluations":
		return fmt.Errorf("agent %q: spec.responseKey %q is reserved", r.ID, r.Spec.ResponseKey)
	}

	if _, err := ParseRestrictedTemplate(r.ID+" spec.body", r.Spec.Body); err != nil {
		return err
	}

	return nil
}

// DefaultREPLTimeout returns the CLI operator-wait default.
func DefaultREPLTimeout() time.Duration {
	return defaultREPLTimeout
}

func (s Spec) InteractiveValue() bool {
	value, err := s.canonicalInteractivePointer()
	if err != nil || value == nil {
		return false
	}

	return *value
}

func (s Spec) MarshalJSON() ([]byte, error) {
	canonical, err := s.canonicalMarshaledSpec()
	if err != nil {
		return nil, err
	}

	return json.Marshal(canonical)
}

func (s Spec) MarshalYAML() (any, error) {
	return s.canonicalMarshaledSpec()
}

func (s Spec) canonicalInteractivePointer() (*bool, error) {
	switch {
	case s.Interactive != nil && s.LegacyREPL != nil && *s.Interactive != *s.LegacyREPL:
		return nil, fmt.Errorf("spec.interactive and deprecated spec.repl must match when both are authored")
	case s.Interactive != nil:
		return boolPointer(*s.Interactive), nil
	case s.LegacyREPL != nil:
		return boolPointer(*s.LegacyREPL), nil
	default:
		return nil, nil
	}
}

func (s Spec) canonicalMarshaledSpec() (specMarshalAlias, error) {
	interactive, err := s.canonicalInteractivePointer()
	if err != nil {
		return specMarshalAlias{}, err
	}

	return specMarshalAlias{
		Description:   s.Description,
		Provider:      s.Provider,
		API:           s.API,
		Evidence:      s.Evidence,
		Questions:     s.Questions,
		Permissions:   s.Permissions,
		Interactive:   interactive,
		Params:        s.Params,
		ResponseKey:   s.ResponseKey,
		Shell:         s.Shell,
		Cwd:           s.Cwd,
		Env:           s.Env,
		Timeout:       s.Timeout,
		OnNonZero:     s.OnNonZero,
		State:         s.State,
		Children:      s.Children,
		Route:         s.Route,
		Body:          s.Body,
		Output:        s.Output,
		MaxIterations: s.MaxIterations,
		OnExhausted:   s.OnExhausted,
	}, nil
}

type specMarshalAlias struct {
	Description   string                 `json:"description"             yaml:"description"`
	Provider      *Provider              `json:"provider,omitempty"      yaml:"provider,omitempty"`
	API           *JevAPI                `json:"api,omitempty"           yaml:"api,omitempty"`
	Evidence      any                    `json:"evidence,omitempty"      yaml:"evidence,omitempty"`
	Questions     map[string]JevQuestion `json:"questions,omitempty"     yaml:"questions,omitempty"`
	Permissions   *Permissions           `json:"permissions,omitempty"   yaml:"permissions,omitempty"`
	Interactive   *bool                  `json:"interactive,omitempty"   yaml:"interactive,omitempty"`
	Params        map[string]string      `json:"params,omitempty"        yaml:"params,omitempty"`
	ResponseKey   string                 `json:"responseKey,omitempty"   yaml:"responseKey,omitempty"`
	Shell         string                 `json:"shell,omitempty"         yaml:"shell,omitempty"`
	Cwd           string                 `json:"cwd,omitempty"           yaml:"cwd,omitempty"`
	Env           map[string]string      `json:"env,omitempty"           yaml:"env,omitempty"`
	Timeout       string                 `json:"timeout,omitempty"       yaml:"timeout,omitempty"`
	OnNonZero     string                 `json:"onNonZero,omitempty"     yaml:"onNonZero,omitempty"`
	State         map[string]any         `json:"state,omitempty"         yaml:"state,omitempty"`
	Children      []Child                `json:"children,omitempty"      yaml:"children,omitempty"`
	Route         string                 `json:"route,omitempty"         yaml:"route,omitempty"`
	Body          string                 `json:"body,omitempty"          yaml:"body,omitempty"`
	Output        string                 `json:"output,omitempty"        yaml:"output,omitempty"`
	MaxIterations *int                   `json:"maxIterations,omitempty" yaml:"maxIterations,omitempty"`
	OnExhausted   string                 `json:"onExhausted,omitempty"   yaml:"onExhausted,omitempty"`
}

func boolPointer(value bool) *bool {
	return &value
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

	allowed := map[string]bool{
		"ref":         true,
		"alias":       true,
		"canEscalate": true,
		"route":       true,
		"default":     true,
		"input":       true,
		"state":       true,
		"params":      true,
	}
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
