package role

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseValidation(t *testing.T) {
	valid := "---\ndescription: test\ntype: codex\nextra_args: [--debug]\n---\nTask: {{ prompt }}\n"

	r, err := Parse("reviewer", []byte(valid))
	if err != nil {
		t.Fatal(err)
	}

	if got, _ := r.Render("{{ ignored }}", nil); !strings.Contains(got, "{{ ignored }}") {
		t.Fatal("prompt was interpreted")
	}

	literal, err := Parse("literal", []byte("---\ndescription: literal example\ntype: codex\n---\nKeep `{{example}}` literal and run {{ prompt }}"))
	if err != nil {
		t.Fatal(err)
	}

	if got, _ := literal.Render("the task", nil); !strings.Contains(got, "{{example}}") || !strings.Contains(got, "the task") {
		t.Fatalf("rendered role = %q", got)
	}

	for _, kind := range SupportedTypes() {
		body := strings.Replace(valid, "type: codex", "type: "+kind, 1)
		if kind == "generic_acp" {
			body = strings.Replace(body, "type: generic_acp", "type: generic_acp\ncmd: /bin/agent", 1)
		}

		_, err := Parse(kind, []byte(body))
		if err != nil {
			t.Errorf("%s: %v", kind, err)
		}
	}

	for name, body := range map[string]string{
		"missing description": "---\ntype: codex\n---\n{{ prompt }}", "missing type": "---\ndescription: x\n---\n{{ prompt }}",
		"gemini": "---\ndescription: x\ntype: gemini\n---\n{{ prompt }}", "unknown": "---\ndescription: x\ntype: codex\nprovider: x\n---\n{{ prompt }}",
		"empty": "---\ndescription: x\ntype: codex\n---\n", "missing expression": "---\ndescription: x\ntype: codex\n---\nhello",
		"duplicate": "---\ndescription: x\ntype: codex\n---\n{{ prompt }} {{ prompt }}", "other expression": "---\ndescription: x\ntype: codex\n---\n{{ name }}",
		"generic cmd": "---\ndescription: x\ntype: generic_acp\n---\n{{ prompt }}",
	} {
		if _, err := Parse(name, []byte(body)); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
}

func TestRenderParametersInOnePass(t *testing.T) {
	data := []byte("---\ndescription: reviewer\ntype: codex\nparams:\n  audience: Intended readers\n  context: Relevant context\n---\n{{ prompt }}\nAudience: {{ audience }}\nAgain: {{audience}}\nContext: {{ context }}\nLiteral: {{ example }}")

	r, err := Parse("reviewer", data)
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
			Description: "reviewer",
			Type:        "codex",
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
			r := Role{ID: test.name, Metadata: Metadata{Description: "test", Type: "codex", Params: test.params}, Template: test.template}
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
	item := Role{ID: "reviewer", Metadata: Metadata{Description: "Reviews changes.", Type: "codex"}, Template: "Review {{ prompt }}"}

	data, err := item.MarshalMarkdown()
	if err != nil {
		t.Fatal(err)
	}

	want := "---\ndescription: Reviews changes.\ntype: codex\n---\n\nReview {{ prompt }}\n"
	if got := string(data); got != want {
		t.Fatalf("Markdown = %q, want %q", got, want)
	}

	parsed, err := Parse("reviewer", data)
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
			Description: "Reviews changes.",
			Type:        "codex",
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

	parsed, err := Parse("reviewer", data)
	if err != nil {
		t.Fatalf("Parse(marshaled role) returned unexpected error: %v", err)
	}

	if !reflect.DeepEqual(parsed.Metadata.Params, item.Metadata.Params) {
		t.Errorf("parsed params = %#v, want %#v", parsed.Metadata.Params, item.Metadata.Params)
	}
}
