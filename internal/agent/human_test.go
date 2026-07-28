package agent

import (
	"strings"
	"testing"
)

func TestHumanValidateAcceptsMinimalResource(t *testing.T) {
	t.Parallel()

	resource := Resource{
		APIVersion: APIVersion,
		Kind:       HumanKind,
		ID:         "workflows/request-input",
		Source:     "workflows/request-input.yaml",
		Spec: Spec{
			Description: "Collects operator input",
			ResponseKey: "answer",
			Body:        "Question: {{ .Input }}",
		},
	}

	if err := resource.Validate(); err != nil {
		t.Fatalf("Resource.Validate() error: %v", err)
	}
}

func TestHumanValidateRejectsInvalidFields(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		spec Spec
		want string
	}{
		{
			name: "missing response key",
			spec: Spec{Description: "collector", Body: "Question: {{ .Input }}"},
			want: "responseKey",
		},
		{
			name: "reserved outputs key",
			spec: Spec{Description: "collector", ResponseKey: "outputs", Body: "Question: {{ .Input }}"},
			want: "reserved",
		},
		{
			name: "reserved scripts key",
			spec: Spec{Description: "collector", ResponseKey: "scripts", Body: "Question: {{ .Input }}"},
			want: "reserved",
		},
		{
			name: "invalid template surface",
			spec: Spec{Description: "collector", ResponseKey: "answer", Body: `Question: {{ .Params.focus }}`},
			want: ".Params is unavailable",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			resource := Resource{
				APIVersion: APIVersion,
				Kind:       HumanKind,
				ID:         "workflows/request-input",
				Source:     "workflows/request-input.yaml",
				Spec:       test.spec,
			}

			err := resource.Validate()
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Resource.Validate() error = %v, want containing %q", err, test.want)
			}
		})
	}
}
