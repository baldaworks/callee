package agent

import (
	"bytes"
	"reflect"
	"strings"
	"testing"
)

func TestDecodeMarkdownKinds(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		body string
		kind Kind
	}{
		{
			name: "role",
			body: "apiVersion: callee.metalagman.dev/v1alpha1\nkind: Role\nspec:\n  description: worker\n  provider:\n    type: codex\n---\nDo this:\n{{ .Input }}\n",
			kind: RoleKind,
		},
		{
			name: "sequential",
			body: "apiVersion: callee.metalagman.dev/v1alpha1\nkind: Sequential\nspec:\n  description: pipeline\n  children: [roles/worker]\n---\n{{ .Input }}\n",
			kind: SequentialKind,
		},
		{
			name: "loop",
			body: "apiVersion: callee.metalagman.dev/v1alpha1\nkind: Loop\nspec:\n  description: goalkeeper\n  children: [roles/worker, roles/validator]\n  maxIterations: 5\n---\n{{ .Input }}\n",
			kind: LoopKind,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			resource, err := DecodeMarkdown("workflows/test", "test.md", []byte("---\n"+test.body))
			if err != nil {
				t.Fatalf("DecodeMarkdown() error: %v", err)
			}

			if resource.Kind != test.kind {
				t.Errorf("resource.Kind = %q, want %q", resource.Kind, test.kind)
			}
		})
	}
}

func TestDecodeYAMLKinds(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		data string
		kind Kind
	}{
		{
			name: "role",
			data: "apiVersion: callee.metalagman.dev/v1alpha1\nkind: Role\nspec:\n  description: worker\n  provider:\n    type: codex\n  body: |\n    Do this:\n    {{ .Input }}\n",
			kind: RoleKind,
		},
		{
			name: "sequential",
			data: "apiVersion: callee.metalagman.dev/v1alpha1\nkind: Sequential\nspec:\n  description: pipeline\n  children: [roles/worker]\n  body: |\n    {{ .Input }}\n",
			kind: SequentialKind,
		},
		{
			name: "loop",
			data: "apiVersion: callee.metalagman.dev/v1alpha1\nkind: Loop\nspec:\n  description: goalkeeper\n  children: [roles/worker, roles/validator]\n  body: |\n    {{ .Input }}\n  maxIterations: 5\n",
			kind: LoopKind,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			resource, err := DecodeYAML("workflows/test", "test.yaml", []byte(test.data))
			if err != nil {
				t.Fatalf("DecodeYAML() error: %v", err)
			}

			if resource.Kind != test.kind {
				t.Errorf("resource.Kind = %q, want %q", resource.Kind, test.kind)
			}

			if resource.Spec.Body == "" {
				t.Error("resource.Spec.Body is empty")
			}
		})
	}
}

func TestDecodeDispatchesSupportedExtensions(t *testing.T) {
	t.Parallel()

	markdown := []byte("---\napiVersion: callee.metalagman.dev/v1alpha1\nkind: Role\nspec:\n  description: worker\n  provider: {type: codex}\n---\n{{ .Input }}\n")
	yamlObject := []byte("apiVersion: callee.metalagman.dev/v1alpha1\nkind: Role\nspec:\n  description: worker\n  provider: {type: codex}\n  body: |\n    {{ .Input }}\n")

	tests := []struct {
		name string
		file string
		data []byte
	}{
		{name: "markdown", file: "worker.md", data: markdown},
		{name: "YAML", file: "worker.yaml", data: yamlObject},
		{name: "short YAML", file: "worker.yml", data: yamlObject},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if _, err := Decode("worker", test.file, test.data); err != nil {
				t.Fatalf("Decode() error: %v", err)
			}
		})
	}

	if _, err := Decode("worker", "worker.json", yamlObject); err == nil || !strings.Contains(err.Error(), "unsupported file extension") {
		t.Fatalf("Decode(.json) error = %v", err)
	}
}

func TestSupportsFileRequiresLowercaseSupportedExtension(t *testing.T) {
	t.Parallel()

	tests := map[string]bool{
		"agent.md":     true,
		"agent.yaml":   true,
		"agent.yml":    true,
		"agent.json":   false,
		"agent.YAML":   false,
		"agent.yml.md": true,
	}

	for path, want := range tests {
		if got := SupportsFile(path); got != want {
			t.Errorf("SupportsFile(%q) = %t, want %t", path, got, want)
		}
	}
}

func TestDecodeYAMLRejectsInvalidDocuments(t *testing.T) {
	t.Parallel()

	valid := "apiVersion: callee.metalagman.dev/v1alpha1\nkind: Role\nspec:\n  description: worker\n  provider: {type: codex}\n  body: |\n    {{ .Input }}\n"
	tests := []struct {
		name string
		data []byte
		want string
	}{
		{name: "empty", data: nil, want: "must contain one document"},
		{name: "invalid UTF-8", data: []byte{0xff}, want: "must be valid UTF-8"},
		{name: "multiple documents", data: []byte(valid + "---\n" + valid), want: "exactly one document"},
		{name: "missing body", data: []byte("apiVersion: callee.metalagman.dev/v1alpha1\nkind: Role\nspec:\n  description: worker\n  provider: {type: codex}\n"), want: "missing property 'body'"},
		{name: "unknown field", data: []byte("apiVersion: callee.metalagman.dev/v1alpha1\nkind: Role\nspec:\n  description: worker\n  provider: {type: codex}\n  prompt: '{{ .Input }}'\n  body: '{{ .Input }}'\n"), want: "unknown YAML field"},
		{name: "invalid template", data: []byte("apiVersion: callee.metalagman.dev/v1alpha1\nkind: Role\nspec:\n  description: worker\n  provider: {type: codex}\n  body: '{{ .Input }} {{ .Output }}'\n"), want: ".Output is available only"},
		{name: "source line", data: []byte("apiVersion: unsupported/v1\nkind: Role\nspec: {}\n"), want: "test.yaml:1:13"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			_, err := DecodeYAML("test", "test.yaml", test.data)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("DecodeYAML() error = %v, want containing %q", err, test.want)
			}
		})
	}
}

func TestMarkdownRoundTripPreservesBody(t *testing.T) {
	t.Parallel()

	body := "Goal:\r\n{{ .Input }}\r\n"
	input := []byte("---\napiVersion: callee.metalagman.dev/v1alpha1\nkind: Sequential\nspec:\n  description: pipeline\n  children:\n    - roles/worker\n---\n" + body)

	resource, err := DecodeMarkdown("workflows/pipeline", "pipeline.md", input)
	if err != nil {
		t.Fatalf("DecodeMarkdown() error: %v", err)
	}

	encoded, err := EncodeMarkdown(resource)
	if err != nil {
		t.Fatalf("EncodeMarkdown() error: %v", err)
	}

	if !bytes.HasSuffix(encoded, []byte(body)) {
		t.Fatalf("EncodeMarkdown() body suffix = %q, want %q", encoded, body)
	}

	roundTrip, err := DecodeMarkdown(resource.ID, resource.Source, encoded)
	if err != nil {
		t.Fatalf("DecodeMarkdown(round trip) error: %v", err)
	}

	if !reflect.DeepEqual(roundTrip, resource) {
		t.Errorf("round trip resource = %#v, want %#v", roundTrip, resource)
	}

	reencoded, err := EncodeMarkdown(roundTrip)
	if err != nil {
		t.Fatalf("EncodeMarkdown(round trip) error: %v", err)
	}

	if !bytes.Equal(reencoded, encoded) {
		t.Errorf("canonical encoding is not idempotent:\nfirst:  %q\nsecond: %q", encoded, reencoded)
	}
}

func TestDecodeMarkdownRejectsFrontmatterBodyAndLegacySyntax(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		data string
		want string
	}{
		{
			name: "frontmatter body",
			data: "---\napiVersion: callee.metalagman.dev/v1alpha1\nkind: Role\nspec:\n  description: worker\n  provider: {type: codex}\n  body: duplicate\n---\n{{ .Input }}",
			want: "frontmatter at test.md:7:9 and as the physical Markdown body beginning at test.md:9:1",
		},
		{
			name: "legacy API",
			data: "---\napi: callee.metalagman.dev\nkind: role\ndescription: worker\nprovider: {type: codex}\n---\n{{ prompt }}",
			want: "missing apiVersion; supported versions",
		},
		{
			name: "unsupported API",
			data: "---\napiVersion: callee.metalagman.dev/v2\nkind: Role\nspec: {}\n---\n{{ .Input }}",
			want: `unsupported apiVersion "callee.metalagman.dev/v2"; supported versions`,
		},
		{
			name: "unsupported kind",
			data: "---\napiVersion: callee.metalagman.dev/v1alpha1\nkind: Parallel\nspec: {}\n---\n{{ .Input }}",
			want: `unsupported kind "Parallel"; supported kinds: Role, Sequential, Loop`,
		},
		{
			name: "wrong field case",
			data: "---\napiVersion: callee.metalagman.dev/v1alpha1\nkind: Loop\nspec:\n  description: loop\n  children: [worker]\n  max_iterations: 2\n---\n{{ .Input }}",
			want: "unknown frontmatter field",
		},
		{
			name: "legacy prompt action",
			data: "---\napiVersion: callee.metalagman.dev/v1alpha1\nkind: Role\nspec:\n  description: worker\n  provider: {type: codex}\n---\n{{ prompt }}",
			want: "use {{ .Prompt }} or {{ .Input }}",
		},
		{
			name: "legacy flat parameter action",
			data: "---\napiVersion: callee.metalagman.dev/v1alpha1\nkind: Role\nspec:\n  description: worker\n  provider: {type: codex}\n  params:\n    focus: Review focus\n---\n{{ .Input }} {{ focus }}",
			want: `use {{ index .Params "focus" }}`,
		},
		{
			name: "unknown child field",
			data: "---\napiVersion: callee.metalagman.dev/v1alpha1\nkind: Sequential\nspec:\n  description: pipeline\n  children:\n    - ref: worker\n      previous: legacy\n---\n{{ .Input }}",
			want: "unknown child field",
		},
		{
			name: "explicit empty enum",
			data: "---\napiVersion: callee.metalagman.dev/v1alpha1\nkind: Loop\nspec:\n  description: loop\n  children: [worker]\n  maxIterations: 2\n  onExhausted: ''\n---\n{{ .Input }}",
			want: "validate schema",
		},
		{
			name: "output outside composite output",
			data: "---\napiVersion: callee.metalagman.dev/v1alpha1\nkind: Role\nspec:\n  description: worker\n  provider: {type: codex}\n---\n{{ .Input }} {{ .Output }}",
			want: ".Output is available only",
		},
		{
			name: "params inside state modifier",
			data: "---\napiVersion: callee.metalagman.dev/v1alpha1\nkind: Role\nspec:\n  description: worker\n  provider: {type: codex}\n  state:\n    focus: '{{ .Params.focus }}'\n---\n{{ .Input }}",
			want: ".Params is unavailable",
		},
		{
			name: "params inside child binding",
			data: "---\napiVersion: callee.metalagman.dev/v1alpha1\nkind: Sequential\nspec:\n  description: pipeline\n  children:\n    - ref: worker\n      params:\n        focus: '{{ .Params.other }}'\n---\n{{ .Input }}",
			want: ".Params is unavailable",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			_, err := DecodeMarkdown("test", "test.md", []byte(test.data))
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("DecodeMarkdown() error = %v, want containing %q", err, test.want)
			}
		})
	}
}
