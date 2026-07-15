package role

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestParseValidation(t *testing.T) {
	valid := "---\ndescription: test\nprovider:\n  type: codex\n  extra_args: [--debug]\n---\nTask: {{ prompt }}\n"

	r, err := parseTestRole("reviewer", []byte(valid))
	if err != nil {
		t.Fatal(err)
	}

	if got, _ := r.Render("{{ ignored }}", nil); !strings.Contains(got, "{{ ignored }}") {
		t.Fatal("prompt was interpreted")
	}

	literal, err := parseTestRole("literal", []byte("---\ndescription: literal example\nprovider:\n  type: codex\n---\nKeep `{{example}}` literal and run {{ prompt }}"))
	if err != nil {
		t.Fatal(err)
	}

	if got, _ := literal.Render("the task", nil); !strings.Contains(got, "{{example}}") || !strings.Contains(got, "the task") {
		t.Fatalf("rendered role = %q", got)
	}

	for _, kind := range SupportedTypes() {
		body := strings.Replace(valid, "type: codex", "type: "+kind, 1)
		if kind == "generic_acp" {
			body = strings.Replace(body, "type: generic_acp", "type: generic_acp\n  cmd: /bin/agent", 1)
		}

		_, err := parseTestRole(kind, []byte(body))
		if err != nil {
			t.Errorf("%s: %v", kind, err)
		}
	}

	for name, body := range map[string]string{
		"missing description": "---\nprovider:\n  type: codex\n---\n{{ prompt }}", "missing type": "---\ndescription: x\nprovider: {}\n---\n{{ prompt }}",
		"gemini": "---\ndescription: x\nprovider:\n  type: gemini\n---\n{{ prompt }}", "unknown": "---\ndescription: x\nprovider:\n  type: codex\n  unknown: x\n---\n{{ prompt }}",
		"empty": "---\ndescription: x\nprovider:\n  type: codex\n---\n", "missing expression": "---\ndescription: x\nprovider:\n  type: codex\n---\nhello",
		"duplicate": "---\ndescription: x\nprovider:\n  type: codex\n---\n{{ prompt }} {{ prompt }}", "other expression": "---\ndescription: x\nprovider:\n  type: codex\n---\n{{ name }}",
		"generic cmd": "---\ndescription: x\nprovider:\n  type: generic_acp\n---\n{{ prompt }}",
	} {
		if _, err := parseTestRole(name, []byte(body)); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
}

func TestParseResourceMetadataAndProviderOptions(t *testing.T) {
	data := []byte("---\ndescription: interactive reviewer\nprovider:\n  type: codex\n  repl: true\n  timeout: 20m\n---\n{{ prompt }}")

	parsed, err := parseTestRole("reviewer", data)
	if err != nil {
		t.Fatal(err)
	}

	if parsed.API() != CurrentAPI || parsed.Kind() != RoleKind {
		t.Fatalf("resource identity = %q/%q", parsed.API(), parsed.Kind())
	}

	if !parsed.Metadata.Provider.REPL {
		t.Fatal("provider.repl = false, want true")
	}

	if got := parsed.PromptTimeout(15 * time.Minute); got != 20*time.Minute {
		t.Fatalf("PromptTimeout() = %s, want 20m", got)
	}
}

func TestParseAcceptsPositiveProviderTimeouts(t *testing.T) {
	for _, timeout := range []string{"15m", "90s", "1h30m", "500ms", "1ns"} {
		t.Run(timeout, func(t *testing.T) {
			body := "---\ndescription: x\nprovider:\n  type: codex\n  timeout: " + timeout + "\n---\n{{ prompt }}"
			if _, err := parseTestRole("reviewer", []byte(body)); err != nil {
				t.Fatalf("Parse() error = %v", err)
			}
		})
	}
}

func TestParseRejectsBreakingSchemaAndInvalidResourceFields(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{name: "flat provider", body: "description: x\ntype: codex", want: "unknown frontmatter field type"},
		{name: "mixed provider", body: "description: x\ntype: codex\nprovider:\n  type: codex", want: "unknown frontmatter field type"},
		{name: "top-level timeout", body: "description: x\ntimeout: 15m\nprovider:\n  type: codex", want: "field timeout"},
		{name: "nested api", body: "description: x\nprovider:\n  type: codex\n  api: example.dev", want: "field api"},
		{name: "nested kind", body: "description: x\nprovider:\n  type: codex\n  kind: role", want: "field kind"},
		{name: "blank api", body: "api: ' '\ndescription: x\nprovider:\n  type: codex", want: "field \"api\" must not be empty"},
		{name: "blank kind", body: "kind: ''\ndescription: x\nprovider:\n  type: codex", want: "field \"kind\" must not be empty"},
		{name: "unsupported api", body: "api: example.dev\ndescription: x\nprovider:\n  type: codex", want: "unsupported api"},
		{name: "unsupported kind", body: "kind: workflow\ndescription: x\nprovider:\n  type: codex", want: "unsupported kind"},
		{name: "unknown provider field", body: "description: x\nprovider:\n  type: codex\n  temperature: 1", want: "field temperature"},
		{name: "blank timeout", body: "description: x\nprovider:\n  type: codex\n  timeout: ''", want: "provider.timeout\" must not be empty"},
		{name: "non-string timeout", body: "description: x\nprovider:\n  type: codex\n  timeout: 15", want: "provider.timeout\" must be a string"},
		{name: "boolean timeout", body: "description: x\nprovider:\n  type: codex\n  timeout: true", want: "provider.timeout\" must be a string"},
		{name: "sequence timeout", body: "description: x\nprovider:\n  type: codex\n  timeout: [15m]", want: "cannot unmarshal"},
		{name: "mapping timeout", body: "description: x\nprovider:\n  type: codex\n  timeout: {value: 15m}", want: "cannot unmarshal"},
		{name: "invalid timeout", body: "description: x\nprovider:\n  type: codex\n  timeout: soon", want: "not a valid duration"},
		{name: "whitespace timeout", body: "description: x\nprovider:\n  type: codex\n  timeout: ' 15m '", want: "not a valid duration"},
		{name: "overflow timeout", body: "description: x\nprovider:\n  type: codex\n  timeout: 999999999999999999999h", want: "not a valid duration"},
		{name: "zero timeout", body: "description: x\nprovider:\n  type: codex\n  timeout: 0s", want: "must be greater than zero"},
		{name: "negative timeout", body: "description: x\nprovider:\n  type: codex\n  timeout: -1s", want: "must be greater than zero"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := parseTestRole(test.name, []byte("---\n"+test.body+"\n---\n{{ prompt }}"))
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Parse() error = %v, want containing %q", err, test.want)
			}
		})
	}
}

func TestParseUsesLoaderKindDefault(t *testing.T) {
	data := []byte("---\ndescription: x\nprovider:\n  type: codex\n---\n{{ prompt }}")

	_, err := Parse("reviewer", data, Defaults{API: CurrentAPI, Kind: "workflow"})
	if err == nil || !strings.Contains(err.Error(), "unsupported kind \"workflow\"") {
		t.Fatalf("Parse() error = %v, want loader kind validation", err)
	}
}

func TestRenderParametersInOnePass(t *testing.T) {
	data := []byte("---\ndescription: reviewer\nprovider:\n  type: codex\nparams:\n  audience: Intended readers\n  context: Relevant context\n---\n{{ prompt }}\nAudience: {{ audience }}\nAgain: {{audience}}\nContext: {{ context }}\nLiteral: {{ example }}")

	r, err := parseTestRole("reviewer", data)
	if err != nil {
		t.Fatalf("Parse(reviewer) returned unexpected error: %v", err)
	}

	got, err := r.Render("Review {{ audience }}", map[string]string{
		"audience": "maintainers",
		"context":  "{{ example }} stays input text",
	})
	if err != nil {
		t.Fatalf("Role.Render() returned unexpected error: %v", err)
	}

	for _, want := range []string{
		"Review {{ audience }}",
		"Audience: maintainers",
		"Again: maintainers",
		"Context: {{ example }} stays input text",
		"Literal: {{ example }}",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("Role.Render() output does not contain %q:\n%s", want, got)
		}
	}
}

func TestRenderParameterErrors(t *testing.T) {
	r := Role{
		ID: "reviewer",
		Metadata: Metadata{
			API:         CurrentAPI,
			Kind:        RoleKind,
			Description: "reviewer",
			Provider:    Provider{Type: "codex"},
			Params:      map[string]string{"audience": "Intended readers"},
		},
		Template: "{{ prompt }} {{ audience }}",
	}

	tests := []struct {
		name   string
		params map[string]string
		want   string
	}{
		{name: "missing", want: "missing=[audience]"},
		{name: "unknown", params: map[string]string{"audience": "team", "extra": "x"}, want: "unknown=[extra]"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := r.Render("review", test.params)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Errorf("Role.Render() error = %v, want containing %q", err, test.want)
			}
		})
	}
}

func TestParameterValidation(t *testing.T) {
	tests := []struct {
		name     string
		params   map[string]string
		template string
		want     string
	}{
		{name: "invalid name", params: map[string]string{"bad.name": "Bad"}, template: "{{ prompt }} {{ bad.name }}", want: "invalid parameter name"},
		{name: "reserved", params: map[string]string{"prompt": "Bad"}, template: "{{ prompt }}", want: "is reserved"},
		{name: "empty description", params: map[string]string{"audience": " "}, template: "{{ prompt }} {{ audience }}", want: "requires a description"},
		{name: "missing expression", params: map[string]string{"audience": "Readers"}, template: "{{ prompt }}", want: "at least once"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			r := Role{ID: test.name, Metadata: Metadata{API: CurrentAPI, Kind: RoleKind, Description: "test", Provider: Provider{Type: "codex"}, Params: test.params}, Template: test.template}
			if err := r.Validate(); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Errorf("Role.Validate() error = %v, want containing %q", err, test.want)
			}
		})
	}
}

func TestRuntimeType(t *testing.T) {
	want := map[string]string{"codex": "codex_acp", "claude": "claude_code_acp", "opencode": "opencode_acp", "copilot": "copilot_acp", "grok": "grok_acp", "generic_acp": "generic_acp"}
	for kind, runtime := range want {
		if got, ok := RuntimeType(kind); !ok || got != runtime {
			t.Errorf("%s = %q", kind, got)
		}
	}

	if _, ok := RuntimeType("gemini"); ok {
		t.Fatal("gemini must not be supported")
	}
}

func TestMarshalMarkdownOmitsEmptyOptionalMetadata(t *testing.T) {
	item := Role{ID: "reviewer", Metadata: Metadata{API: CurrentAPI, Kind: RoleKind, Description: "Reviews changes.", Provider: Provider{Type: "codex"}}, Template: "Review {{ prompt }}"}

	data, err := item.MarshalMarkdown()
	if err != nil {
		t.Fatal(err)
	}

	want := "---\napi: callee.metalagman.dev\nkind: role\ndescription: Reviews changes.\nprovider:\n    type: codex\n---\n\nReview {{ prompt }}\n"
	if got := string(data); got != want {
		t.Fatalf("Markdown = %q, want %q", got, want)
	}

	parsed, err := parseTestRole("reviewer", data)
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(parsed.Metadata, item.Metadata) {
		t.Fatalf("metadata = %#v, want %#v", parsed.Metadata, item.Metadata)
	}

	if parsed.Template != "\nReview {{ prompt }}\n" {
		t.Fatalf("template = %q", parsed.Template)
	}
}

func TestMarshalMarkdownIncludesParameterDescriptions(t *testing.T) {
	item := Role{
		ID: "reviewer",
		Metadata: Metadata{
			API:         CurrentAPI,
			Kind:        RoleKind,
			Description: "Reviews changes.",
			Provider:    Provider{Type: "codex"},
			Params:      map[string]string{"audience": "Intended readers"},
		},
		Template: "Review {{ prompt }} for {{ audience }}",
	}

	data, err := item.MarshalMarkdown()
	if err != nil {
		t.Fatalf("Role.MarshalMarkdown() returned unexpected error: %v", err)
	}

	if !strings.Contains(string(data), "params:\n    audience: Intended readers\n") {
		t.Errorf("Role.MarshalMarkdown() output does not contain params:\n%s", data)
	}

	parsed, err := parseTestRole("reviewer", data)
	if err != nil {
		t.Fatalf("Parse(marshaled role) returned unexpected error: %v", err)
	}

	if !reflect.DeepEqual(parsed.Metadata.Params, item.Metadata.Params) {
		t.Errorf("parsed params = %#v, want %#v", parsed.Metadata.Params, item.Metadata.Params)
	}
}

func parseTestRole(id string, data []byte) (Role, error) {
	return Parse(id, data, Defaults{API: CurrentAPI, Kind: RoleKind})
}
