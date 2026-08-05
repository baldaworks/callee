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
			name: "script",
			body: "apiVersion: callee.metalagman.dev/v1alpha1\nkind: Script\nspec:\n  description: validator\n---\necho {{ .Input }}\n",
			kind: ScriptKind,
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
		{
			name: "router",
			body: "apiVersion: callee.metalagman.dev/v1alpha1\nkind: Router\nspec:\n  description: dispatcher\n  route: '{{ .Input }}'\n  children:\n    - route: default\n      ref: roles/worker\n    - default: true\n      ref: roles/fallback\n---\n{{ .Prompt }}\n",
			kind: RouterKind,
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
			name: "script",
			data: "apiVersion: callee.metalagman.dev/v1alpha1\nkind: Script\nspec:\n  description: validator\n  body: |\n    echo {{ .Input }}\n",
			kind: ScriptKind,
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
		{
			name: "router",
			data: `apiVersion: callee.metalagman.dev/v1alpha1
kind: Router
spec:
  description: dispatcher
  route: '{{ .Input }}'
  children:
    - route: default
      ref: roles/worker
    - default: true
      ref: roles/fallback
  body: |
    {{ .Prompt }}
`,
			kind: RouterKind,
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

func TestDecodeChildCanEscalate(t *testing.T) {
	t.Parallel()

	data := []byte(`---
apiVersion: callee.metalagman.dev/v1alpha1
kind: Loop
spec:
  description: loop
  children:
    - ref: roles/worker
      canEscalate: true
    - roles/reviewer
  maxIterations: 2
---
{{ .Input }}
`)

	resource, err := DecodeMarkdown("workflows/loop", "loop.md", data)
	if err != nil {
		t.Fatalf("DecodeMarkdown() error: %v", err)
	}

	if !resource.Spec.Children[0].CanEscalate {
		t.Error("mapped child CanEscalate = false, want true")
	}

	if resource.Spec.Children[1].CanEscalate {
		t.Error("scalar child CanEscalate = true, want default false")
	}
}

func TestDecodeRouterEdges(t *testing.T) {
	t.Parallel()

	data := []byte(`---
apiVersion: callee.metalagman.dev/v1alpha1
kind: Router
spec:
  description: dispatcher
  route: '{{ .Input }}'
  children:
    - route: default
      ref: roles/default-key
    - default: true
      ref: roles/fallback
---
{{ .Prompt }}
`)

	resource, err := DecodeMarkdown("workflows/router", "router.md", data)
	if err != nil {
		t.Fatalf("DecodeMarkdown() error: %v", err)
	}

	if got, want := resource.Spec.Children[0].Route, "default"; got != want {
		t.Errorf("named child route = %q, want %q", got, want)
	}

	if resource.Spec.Children[0].Default {
		t.Error("named child Default = true, want false")
	}

	if !resource.Spec.Children[1].Default || resource.Spec.Children[1].Route != "" {
		t.Errorf("fallback child = %+v, want default without named route", resource.Spec.Children[1])
	}
}

func TestDecodeRouterRejectsInvalidEdges(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		children string
		want     string
	}{
		{name: "scalar child", children: "    - roles/worker\n", want: "validate schema"},
		{name: "missing selector", children: "    - ref: roles/worker\n", want: "validate schema"},
		{name: "blank route", children: "    - ref: roles/worker\n      route: ''\n", want: "validate schema"},
		{name: "route and default", children: "    - ref: roles/worker\n      route: work\n      default: true\n", want: "validate schema"},
		{name: "false default", children: "    - ref: roles/worker\n      default: false\n", want: "validate schema"},
		{name: "duplicate route", children: "    - ref: roles/first\n      route: work\n    - ref: roles/second\n      route: work\n", want: "duplicates"},
		{name: "multiple defaults", children: "    - ref: roles/first\n      default: true\n    - ref: roles/second\n      default: true\n", want: "duplicates"},
		{name: "noncanonical route", children: "    - ref: roles/worker\n      route: ' work '\n", want: "leading or trailing whitespace"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			data := "apiVersion: callee.metalagman.dev/v1alpha1\nkind: Router\nspec:\n  description: dispatcher\n  route: '{{ .Input }}'\n  children:\n" + test.children + "  body: '{{ .Prompt }}'\n"

			_, err := DecodeYAML("workflows/router", "router.yaml", []byte(data))
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("DecodeYAML() error = %v, want containing %q", err, test.want)
			}
		})
	}
}

func TestDecodeRolePermissions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		file string
		data string
		want PermissionMode
	}{
		{
			name: "Markdown allow",
			file: "worker.md",
			data: "---\napiVersion: callee.metalagman.dev/v1alpha1\nkind: Role\nspec:\n  description: worker\n  provider: {type: codex}\n  permissions: {mode: allow}\n---\n{{ .Input }}\n",
			want: PermissionModeAllow,
		},
		{
			name: "YAML deny",
			file: "worker.yaml",
			data: "apiVersion: callee.metalagman.dev/v1alpha1\nkind: Role\nspec:\n  description: worker\n  provider: {type: codex}\n  permissions: {mode: deny}\n  body: '{{ .Input }}'\n",
			want: PermissionModeDeny,
		},
		{
			name: "omitted defaults to ask",
			file: "worker.yml",
			data: "apiVersion: callee.metalagman.dev/v1alpha1\nkind: Role\nspec:\n  description: worker\n  provider: {type: codex}\n  body: '{{ .Input }}'\n",
			want: PermissionModeAsk,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got, err := Decode("roles/worker", test.file, []byte(test.data))
			if err != nil {
				t.Fatalf("Decode() error: %v", err)
			}

			if mode := got.EffectivePermissionMode(); mode != test.want {
				t.Errorf("EffectivePermissionMode() = %q, want %q", mode, test.want)
			}
		})
	}
}

func TestDecodeRoleInteractiveCompat(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		file string
		data string
		want bool
	}{
		{
			name: "interactive only",
			file: "worker.yaml",
			data: "apiVersion: callee.metalagman.dev/v1alpha1\nkind: Role\nspec:\n  description: worker\n  provider: {type: codex}\n  interactive: true\n  body: '{{ .Input }}'\n",
			want: true,
		},
		{
			name: "legacy repl only",
			file: "worker.yaml",
			data: "apiVersion: callee.metalagman.dev/v1alpha1\nkind: Role\nspec:\n  description: worker\n  provider: {type: codex}\n  repl: true\n  body: '{{ .Input }}'\n",
			want: true,
		},
		{
			name: "both same value",
			file: "worker.yaml",
			data: "apiVersion: callee.metalagman.dev/v1alpha1\nkind: Role\nspec:\n  description: worker\n  provider: {type: codex}\n  interactive: true\n  repl: true\n  body: '{{ .Input }}'\n",
			want: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got, err := Decode("roles/worker", test.file, []byte(test.data))
			if err != nil {
				t.Fatalf("Decode() error: %v", err)
			}

			if interactive := got.Interactive(); interactive != test.want {
				t.Fatalf("Interactive() = %t, want %t", interactive, test.want)
			}

			if encoded, err := EncodeMarkdown(got); err != nil {
				t.Fatalf("EncodeMarkdown() error: %v", err)
			} else if strings.Contains(string(encoded), "\nrepl:") || !strings.Contains(string(encoded), "interactive: true") {
				t.Fatalf("EncodeMarkdown() = %q, want canonical interactive field only", string(encoded))
			}
		})
	}
}

func TestDecodeRejectsConflictingInteractiveCompatFields(t *testing.T) {
	t.Parallel()

	data := "apiVersion: callee.metalagman.dev/v1alpha1\nkind: Role\nspec:\n  description: worker\n  provider: {type: codex}\n  interactive: true\n  repl: false\n  body: '{{ .Input }}'\n"

	if _, err := DecodeYAML("roles/worker", "worker.yaml", []byte(data)); err == nil || !strings.Contains(err.Error(), "spec.interactive and deprecated spec.repl must match") {
		t.Fatalf("DecodeYAML(conflicting interactive/repl) error = %v", err)
	}
}

func TestDecodeRejectsInvalidPermissionPlacementAndMode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		data string
		want string
	}{
		{
			name: "invalid mode",
			data: "---\napiVersion: callee.metalagman.dev/v1alpha1\nkind: Role\nspec:\n  description: worker\n  provider: {type: codex}\n  permissions: {mode: always}\n---\n{{ .Input }}\n",
			want: "validate schema",
		},
		{
			name: "blank mode",
			data: "---\napiVersion: callee.metalagman.dev/v1alpha1\nkind: Role\nspec:\n  description: worker\n  provider: {type: codex}\n  permissions: {mode: ''}\n---\n{{ .Input }}\n",
			want: "validate schema",
		},
		{
			name: "differently cased mode",
			data: "---\napiVersion: callee.metalagman.dev/v1alpha1\nkind: Role\nspec:\n  description: worker\n  provider: {type: codex}\n  permissions: {mode: Allow}\n---\n{{ .Input }}\n",
			want: "validate schema",
		},
		{
			name: "missing mode",
			data: "---\napiVersion: callee.metalagman.dev/v1alpha1\nkind: Role\nspec:\n  description: worker\n  provider: {type: codex}\n  permissions: {}\n---\n{{ .Input }}\n",
			want: "validate schema",
		},
		{
			name: "additional property",
			data: "---\napiVersion: callee.metalagman.dev/v1alpha1\nkind: Role\nspec:\n  description: worker\n  provider: {type: codex}\n  permissions: {mode: ask, tools: allow}\n---\n{{ .Input }}\n",
			want: "unknown frontmatter field tools",
		},
		{
			name: "provider placement",
			data: "---\napiVersion: callee.metalagman.dev/v1alpha1\nkind: Role\nspec:\n  description: worker\n  provider: {type: codex, permissions: {mode: allow}}\n---\n{{ .Input }}\n",
			want: "unknown frontmatter field permissions",
		},
		{
			name: "composite placement",
			data: "---\napiVersion: callee.metalagman.dev/v1alpha1\nkind: Sequential\nspec:\n  description: pipeline\n  children: [roles/worker]\n  permissions: {mode: deny}\n---\n{{ .Input }}\n",
			want: "validate schema",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if _, err := DecodeMarkdown("test", "test.md", []byte(test.data)); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("DecodeMarkdown() error = %v, want containing %q", err, test.want)
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

func TestIsDiscoverableResourceFileClassifiesContent(t *testing.T) {
	t.Parallel()

	currentMarkdown := "---\napiVersion: callee.metalagman.dev/v1alpha1\nkind: Role\nspec: {}\n---\n{{ .Input }}\n"
	currentYAML := "apiVersion: callee.metalagman.dev/v1alpha1\nkind: Role\nspec: {}\nbody: '{{ .Input }}'\n"

	tests := []struct {
		name    string
		source  string
		data    []byte
		want    bool
		wantErr string
	}{
		{name: "plain Markdown documentation", source: "README.md", data: []byte("# Catalog\n"), want: false},
		{name: "Markdown without API version", source: "guide.md", data: []byte("---\ntitle: Guide\n---\nText\n"), want: false},
		{name: "Markdown with non-current API version", source: "legacy.md", data: []byte("---\napiVersion: example.dev/v1\n---\nText\n"), want: false},
		{name: "current Markdown resource", source: "roles/worker.md", data: []byte(currentMarkdown), want: true},
		{name: "unclosed Markdown frontmatter", source: "broken.md", data: []byte("---\napiVersion: callee.metalagman.dev/v1alpha1\n"), wantErr: "closing delimiter"},
		{name: "invalid Markdown frontmatter", source: "broken.md", data: []byte("---\ntitle: [\n---\nText\n"), wantErr: "frontmatter"},
		{name: "YAML without API version", source: "guide.yaml", data: []byte("title: Guide\n"), want: false},
		{name: "YML with non-current API version", source: "legacy.yml", data: []byte("apiVersion: example.dev/v1\n"), want: false},
		{name: "current YAML resource", source: "roles/worker.yaml", data: []byte(currentYAML), want: true},
		{name: "invalid YAML", source: "broken.yaml", data: []byte("title: [\n"), wantErr: "decode YAML document"},
		{name: "multiple YAML documents", source: "broken.yml", data: []byte("title: first\n---\ntitle: second\n"), wantErr: "exactly one document"},
		{name: "unsupported extension", source: "guide.txt", data: []byte("# Catalog\n"), want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got, err := IsDiscoverableResourceFile(test.source, test.data)
			if test.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantErr) {
					t.Fatalf("IsDiscoverableResourceFile() error = %v, want containing %q", err, test.wantErr)
				}

				return
			}

			if err != nil {
				t.Fatalf("IsDiscoverableResourceFile() error = %v", err)
			}

			if got != test.want {
				t.Errorf("IsDiscoverableResourceFile() = %t, want %t", got, test.want)
			}
		})
	}
}

func TestDecodeRemainsStrictForSkippedDiscoveryDocuments(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		source string
		data   []byte
		want   string
	}{
		{name: "plain Markdown", source: "README.md", data: []byte("# Catalog\n"), want: "opening delimiter"},
		{name: "Markdown without API version", source: "guide.md", data: []byte("---\ntitle: Guide\n---\nText\n"), want: "missing apiVersion"},
		{name: "YAML without API version", source: "guide.yaml", data: []byte("title: Guide\n"), want: "missing apiVersion"},
		{name: "YML with non-current API version", source: "legacy.yml", data: []byte("apiVersion: example.dev/v1\n"), want: "unsupported apiVersion"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if _, err := Decode("document", test.source, test.data); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Errorf("Decode() error = %v, want containing %q", err, test.want)
			}
		})
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

func TestDecodeYAMLHumanResource(t *testing.T) {
	t.Parallel()

	data := []byte("apiVersion: callee.metalagman.dev/v1alpha1\nkind: Human\nspec:\n  description: Ask the operator\n  responseKey: approval\n  body: |\n    Question: {{ .Input }}\n")

	got, err := DecodeYAML("workflows/request-input", "request-input.yaml", data)
	if err != nil {
		t.Fatalf("DecodeYAML() error: %v", err)
	}

	if got.Kind != HumanKind {
		t.Fatalf("DecodeYAML() kind = %q, want %q", got.Kind, HumanKind)
	}

	if got.Spec.ResponseKey != "approval" {
		t.Fatalf("DecodeYAML() responseKey = %q, want approval", got.Spec.ResponseKey)
	}
}

func TestDecodeYAMLHumanRequiresResponseKey(t *testing.T) {
	t.Parallel()

	data := []byte("apiVersion: callee.metalagman.dev/v1alpha1\nkind: Human\nspec:\n  description: Ask the operator\n  body: |\n    Question: {{ .Input }}\n")

	_, err := DecodeYAML("workflows/request-input", "request-input.yaml", data)
	if err == nil || !strings.Contains(err.Error(), "missing property 'responseKey'") {
		t.Fatalf("DecodeYAML() error = %v, want missing responseKey", err)
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

func TestRouterMarkdownRoundTripPreservesEdges(t *testing.T) {
	t.Parallel()

	input := []byte(`---
apiVersion: callee.metalagman.dev/v1alpha1
kind: Router
spec:
  description: dispatcher
  route: '{{ .State.route }}'
  children:
    - route: work
      ref: roles/worker
      alias: worker
    - default: true
      ref: roles/fallback
      alias: fallback
---
{{ .Prompt }}
`)

	resource, err := DecodeMarkdown("workflows/router", "router.md", input)
	if err != nil {
		t.Fatalf("DecodeMarkdown() error: %v", err)
	}

	encoded, err := EncodeMarkdown(resource)
	if err != nil {
		t.Fatalf("EncodeMarkdown() error: %v", err)
	}

	roundTrip, err := DecodeMarkdown(resource.ID, resource.Source, encoded)
	if err != nil {
		t.Fatalf("DecodeMarkdown(round trip) error: %v", err)
	}

	if !reflect.DeepEqual(roundTrip, resource) {
		t.Errorf("round trip resource = %#v, want %#v", roundTrip, resource)
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
			want: `unsupported kind "Parallel"; supported kinds: Role, Script, Human, Sequential, Loop, Router`,
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
