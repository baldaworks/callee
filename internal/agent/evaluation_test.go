package agent

import (
	"strings"
	"testing"
	"time"
)

func TestEvaluationResourceDefaultsAndValidation(t *testing.T) {
	t.Parallel()

	typesafe := validEvaluationResource(TypeSafeJevKind)
	if err := typesafe.Validate(); err != nil {
		t.Fatalf("Validate(TypeSafeJev) error = %v", err)
	}

	if got := typesafe.EvaluationModel(); got != "" {
		t.Fatalf("EvaluationModel() = %q, want empty for adapter resolution", got)
	}

	if got, want := typesafe.EvaluationTimeout(), 30*time.Second; got != want {
		t.Fatalf("EvaluationTimeout() = %v, want %v", got, want)
	}

	openrouter := validEvaluationResource(OpenRouterDecisionKind)
	openrouter.Spec.Model = "openai/gpt-decision"

	if err := openrouter.Validate(); err != nil {
		t.Fatalf("Validate(OpenRouterDecision) error = %v", err)
	}
}

func TestDecodeMarkdownOpenRouterDecisionWithStructuredEvidence(t *testing.T) {
	t.Parallel()

	data := []byte(`---
apiVersion: callee.metalagman.dev/v1alpha1
kind: OpenRouterDecision
spec:
  description: Classify the request
  model: typesafe/jev-1.13
  evidence:
    request: "{{ .Input }}"
    retry: false
    note: null
  questions:
    urgent:
      type: noul
      instructions: Is this urgent?
      criteria:
        "true": Time-sensitive
        "false": Can wait
---
`)

	resource, err := DecodeMarkdown("judge", "judge.md", data)
	if err != nil {
		t.Fatalf("DecodeMarkdown() error = %v", err)
	}

	if resource.Spec.Body != "" {
		t.Fatalf("Spec.Body = %q, want empty", resource.Spec.Body)
	}

	evidence, ok := resource.Spec.Evidence.(map[string]any)
	if !ok || evidence["retry"] != false || evidence["note"] != nil {
		t.Fatalf("Spec.Evidence = %#v", resource.Spec.Evidence)
	}

	encoded, err := EncodeMarkdown(resource)
	if err != nil {
		t.Fatalf("EncodeMarkdown() error = %v", err)
	}

	decoded, err := DecodeMarkdown("judge", "judge.md", encoded)
	if err != nil {
		t.Fatalf("DecodeMarkdown(round trip) error = %v", err)
	}

	if decoded.EvaluationModel() != "typesafe/jev-1.13" {
		t.Fatalf("EvaluationModel(round trip) = %q", decoded.EvaluationModel())
	}
}

func TestEvaluationCleanBreakRejectsLegacyJevAndAPISelector(t *testing.T) {
	t.Parallel()

	legacyKind := []byte("---\napiVersion: callee.metalagman.dev/v1alpha1\nkind: Jev\nspec: {}\n---\n")
	if _, err := DecodeMarkdown("legacy", "legacy.md", legacyKind); err == nil || !strings.Contains(err.Error(), `unsupported kind "Jev"`) {
		t.Fatalf("DecodeMarkdown(legacy kind) error = %v", err)
	}

	legacyAPI := []byte(`---
apiVersion: callee.metalagman.dev/v1alpha1
kind: TypeSafeJev
spec:
  description: Legacy selector
  api:
    type: typesafe
  questions:
    safe:
      type: noul
      instructions: Safe?
---
evidence
`)
	if _, err := DecodeMarkdown("legacy-api", "legacy-api.md", legacyAPI); err == nil || !strings.Contains(err.Error(), "api") {
		t.Fatalf("DecodeMarkdown(legacy api) error = %v", err)
	}
}

func TestEvaluationResourceRejectsInvalidContracts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		kind Kind
		edit func(*Resource)
		want string
	}{
		{name: "both evidence sources", kind: TypeSafeJevKind, edit: func(r *Resource) { r.Spec.Evidence = map[string]any{"x": "y"} }, want: "exactly one"},
		{name: "no evidence", kind: TypeSafeJevKind, edit: func(r *Resource) { r.Spec.Body = "" }, want: "exactly one"},
		{name: "wrong native model", kind: TypeSafeJevKind, edit: func(r *Resource) { r.Spec.Model = "gpt-5" }, want: "TypeSafe Jev model"},
		{name: "missing OpenRouter model", kind: OpenRouterDecisionKind, edit: func(r *Resource) { r.Spec.Model = "" }, want: "requires nonblank spec.model"},
		{name: "generic OpenRouter model", kind: OpenRouterDecisionKind, edit: func(r *Resource) { r.Spec.Model = "openai/gpt-decision" }},
		{name: "invalid timeout", kind: TypeSafeJevKind, edit: func(r *Resource) { r.Spec.Timeout = "0s" }, want: "greater than zero"},
		{name: "blank questions", kind: TypeSafeJevKind, edit: func(r *Resource) { r.Spec.Questions = nil }, want: "at least one"},
		{name: "OpenRouter partial noul criteria", kind: OpenRouterDecisionKind, edit: func(r *Resource) {
			r.Spec.Model = "vendor/model"
			r.Spec.Questions["urgent"] = EvaluationQuestion{Type: "noul", Instructions: "Urgent?", Criteria: map[string]any{"true": "yes"}}
		}, want: "both true and false"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			resource := validEvaluationResource(test.kind)

			if test.kind == OpenRouterDecisionKind {
				resource.Spec.Model = "typesafe/jev-1.13"
				question := resource.Spec.Questions["urgent"]
				question.Criteria = map[string]any{"true": "yes", "false": "no"}
				resource.Spec.Questions["urgent"] = question
			}

			test.edit(&resource)

			err := resource.validateEvaluation()
			if test.want == "" && err != nil {
				t.Fatalf("Validate() error = %v", err)
			}

			if test.want != "" && (err == nil || !strings.Contains(err.Error(), test.want)) {
				t.Fatalf("Validate() error = %v, want containing %q", err, test.want)
			}
		})
	}
}

func validEvaluationResource(kind Kind) Resource {
	return Resource{
		APIVersion: APIVersion,
		Kind:       kind,
		ID:         "judge",
		Spec: Spec{
			Description: "Judge urgency",
			Body:        "{{ .Input }}",
			Questions: map[string]EvaluationQuestion{
				"urgent": {Type: "noul", Instructions: "Is this urgent?"},
			},
		},
	}
}
