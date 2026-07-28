package agent

import (
	"strings"
	"testing"
)

func TestScriptValidateAcceptsMinimalResource(t *testing.T) {
	t.Parallel()

	resource := Resource{
		APIVersion: APIVersion,
		Kind:       ScriptKind,
		ID:         "scripts/validator",
		Source:     "scripts/validator.yaml",
		Spec: Spec{
			Description: "Runs a validator",
			Body:        "echo {{ .Input }}",
		},
	}

	if err := resource.Validate(); err != nil {
		t.Fatalf("Resource.Validate() error: %v", err)
	}
}

func TestScriptValidateRejectsInvalidFields(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		spec Spec
		want string
	}{
		{
			name: "bad shell",
			spec: Spec{Description: "validator", Shell: "pwsh", Body: "echo ok"},
			want: "spec.shell",
		},
		{
			name: "bad timeout",
			spec: Spec{Description: "validator", Timeout: "later", Body: "echo ok"},
			want: "spec.timeout",
		},
		{
			name: "non-positive timeout",
			spec: Spec{Description: "validator", Timeout: "0s", Body: "echo ok"},
			want: "greater than zero",
		},
		{
			name: "bad non-zero policy",
			spec: Spec{Description: "validator", OnNonZero: "ignore", Body: "echo ok"},
			want: "validate schema",
		},
		{
			name: "env name contains equals",
			spec: Spec{Description: "validator", Env: map[string]string{"BAD=NAME": "x"}, Body: "echo ok"},
			want: "must not contain =",
		},
		{
			name: "reserved scripts state",
			spec: Spec{Description: "validator", State: map[string]any{"scripts": map[string]any{}}, Body: "echo ok"},
			want: "validate schema",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			resource := Resource{
				APIVersion: APIVersion,
				Kind:       ScriptKind,
				ID:         "scripts/validator",
				Source:     "scripts/validator.yaml",
				Spec:       test.spec,
			}

			err := resource.Validate()
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Resource.Validate() error = %v, want containing %q", err, test.want)
			}
		})
	}
}
