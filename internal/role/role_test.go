package role

import (
	"strings"
	"testing"
)

func TestParseValidation(t *testing.T) {
	valid := "---\ndescription: test\ntype: codex\nextra_args: [--debug]\n---\nTask: {{ prompt }}\n"

	r, err := Parse("reviewer", []byte(valid))
	if err != nil {
		t.Fatal(err)
	}

	if got, _ := r.Render("{{ ignored }}"); !strings.Contains(got, "{{ ignored }}") {
		t.Fatal("prompt was interpreted")
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
