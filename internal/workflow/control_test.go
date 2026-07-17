package workflow

import (
	"strings"
	"testing"
)

func TestParseResponse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		content  string
		repl     bool
		outcome  outcome
		artifact string
		wantErr  string
	}{
		{name: "plain return", content: "artifact", outcome: outcomeReturn, artifact: "artifact"},
		{name: "explicit return", content: "artifact\n\n" + controlReturn + "\n", repl: true, outcome: outcomeReturn, artifact: "artifact"},
		{name: "await", content: "Question?\r\n\r\n" + controlAwait + "\r\n", repl: true, outcome: outcomeAwait, artifact: "Question?"},
		{name: "empty escalation", content: controlEscalate, outcome: outcomeEscalate},
		{name: "failure detail", content: "bad result\n\n" + controlFail, outcome: outcomeFail, artifact: "bad result"},
		{name: "non-REPL await", content: "Question?\n\n" + controlAwait, wantErr: "non-REPL"},
		{name: "missing repl control", content: "artifact", repl: true, wantErr: "missing"},
		{name: "malformed reserved", content: "artifact\n\ncallee.control.v1.stop", wantErr: "malformed"},
		{name: "missing separator", content: "artifact\n" + controlReturn, repl: true, wantErr: "separated"},
		{name: "extra LF separator", content: "artifact\n\n\n" + controlReturn, repl: true, wantErr: "exactly one"},
		{name: "extra CRLF separator", content: "artifact\r\n\r\n\r\n" + controlReturn, repl: true, wantErr: "exactly one"},
		{name: "empty return", content: controlReturn, repl: true, wantErr: "nonempty"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got, err := parseResponse(test.content, test.repl)
			if test.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantErr) {
					t.Fatalf("parseResponse() error = %v, want containing %q", err, test.wantErr)
				}

				return
			}

			if err != nil {
				t.Fatalf("parseResponse() error: %v", err)
			}

			if got.outcome != test.outcome || got.artifact != test.artifact {
				t.Errorf("parseResponse() = %+v, want outcome=%v artifact=%q", got, test.outcome, test.artifact)
			}
		})
	}
}

func TestControlInstructionsScopeEscalationToLoopDescendants(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name          string
		repl          bool
		canEscalate   bool
		wantEscalate  bool
		wantMandatory []string
	}{
		{name: "root one shot", wantMandatory: []string{"Return the requested artifact normally.", controlFail}},
		{name: "loop one shot", canEscalate: true, wantEscalate: true, wantMandatory: []string{"Return the requested artifact normally.", controlFail}},
		{name: "root repl", repl: true, wantMandatory: []string{controlAwait, controlReturn, controlFail}},
		{name: "loop repl", repl: true, canEscalate: true, wantEscalate: true, wantMandatory: []string{controlAwait, controlReturn, controlFail}},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got := controlInstructions(test.repl, test.canEscalate)
			if strings.Contains(got, controlEscalate) != test.wantEscalate {
				t.Errorf("controlInstructions() escalation presence = %t, want %t:\n%s", strings.Contains(got, controlEscalate), test.wantEscalate, got)
			}

			for _, want := range test.wantMandatory {
				if !strings.Contains(got, want) {
					t.Errorf("controlInstructions() is missing %q:\n%s", want, got)
				}
			}
		})
	}
}
