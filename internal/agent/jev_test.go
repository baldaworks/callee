package agent

import (
	"strings"
	"testing"
	"time"
)

func TestJevResourceDefaultsAndValidation(t *testing.T) {
	t.Parallel()

	resource := validJevResource()
	if err := resource.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}

	if got, want := resource.JevModel(), "jev-latest"; got != want {
		t.Fatalf("JevModel() = %q, want %q", got, want)
	}

	if got, want := resource.JevTimeout(), 30*time.Second; got != want {
		t.Fatalf("JevTimeout() = %v, want %v", got, want)
	}

	resource.Spec.API.Type = "openrouter"
	resource.Spec.API.Model = ""

	if err := resource.Validate(); err != nil {
		t.Fatalf("Validate(openrouter) error = %v", err)
	}

	if got, want := resource.JevModel(), "~typesafe/jev-latest"; got != want {
		t.Fatalf("JevModel(openrouter) = %q, want %q", got, want)
	}
}

func TestDecodeMarkdownJevWithStructuredEvidence(t *testing.T) {
	t.Parallel()

	data := []byte(`---
apiVersion: callee.metalagman.dev/v1alpha1
kind: Jev
spec:
  description: Classify the request
  api:
    type: openrouter
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

	if decoded.JevModel() != "typesafe/jev-1.13" {
		t.Fatalf("JevModel(round trip) = %q", decoded.JevModel())
	}
}

func TestJevResourceRejectsInvalidContracts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		edit   func(*Resource)
		want   string
		common bool
	}{
		{name: "both evidence sources", edit: func(r *Resource) { r.Spec.Evidence = map[string]any{"x": "y"} }, want: "exactly one"},
		{name: "no evidence", edit: func(r *Resource) { r.Spec.Body = "" }, want: "exactly one"},
		{name: "unknown API", edit: func(r *Resource) { r.Spec.API.Type = "chat" }, want: "must be typesafe or openrouter"},
		{name: "wrong native model", edit: func(r *Resource) { r.Spec.API.Model = "gpt-5" }, want: "TypeSafe Jev model"},
		{name: "wrong OpenRouter model", edit: func(r *Resource) { r.Spec.API.Type = "openrouter"; r.Spec.API.Model = "openai/gpt-5" }, want: "OpenRouter TypeSafe Jev model"},
		{name: "invalid timeout", edit: func(r *Resource) { r.Spec.API.Timeout = "0s" }, want: "greater than zero"},
		{name: "blank questions", edit: func(r *Resource) { r.Spec.Questions = nil }, want: "at least one"},
		{name: "choice cardinality", edit: func(r *Resource) {
			r.Spec.Questions["choice"] = JevQuestion{Type: "choice", Instructions: "Pick", Criteria: map[string]any{"only": "one"}}
		}, want: "2 to 255"},
		{name: "score cardinality", edit: func(r *Resource) {
			r.Spec.Questions["score"] = JevQuestion{Type: "score", Instructions: "Rate", Criteria: []any{"only"}}
		}, want: "2 to 10"},
		{name: "OpenRouter partial noul criteria", edit: func(r *Resource) {
			r.Spec.API.Type = "openrouter"
			r.Spec.Questions["urgent"] = JevQuestion{Type: "noul", Instructions: "Urgent?", Criteria: map[string]any{"true": "yes"}}
		}, want: "both true and false"},
		{name: "reserved evaluations", edit: func(r *Resource) { r.Spec.State = map[string]any{"evaluations": map[string]any{}} }, want: "evaluations is reserved", common: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			resource := validJevResource()
			test.edit(&resource)

			var err error
			if test.common {
				err = resource.validateCommon()
			} else {
				err = resource.validateJev()
			}

			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Validate() error = %v, want containing %q", err, test.want)
			}
		})
	}
}

func validJevResource() Resource {
	return Resource{
		APIVersion: APIVersion,
		Kind:       JevKind,
		ID:         "judge",
		Spec: Spec{
			Description: "Judge urgency",
			API:         &JevAPI{Type: "typesafe"},
			Body:        "{{ .Input }}",
			Questions: map[string]JevQuestion{
				"urgent": {
					Type:         "noul",
					Instructions: "Is this urgent?",
				},
			},
		},
	}
}
