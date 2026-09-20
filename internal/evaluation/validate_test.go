package evaluation

import (
	"math"
	"strings"
	"testing"

	"github.com/baldaworks/callee/internal/agent"
)

func TestValidateResultAcceptsAllAnswerTypesAndRounding(t *testing.T) {
	t.Parallel()

	request := validRequest()
	result := validResult()

	if err := ValidateRequest(request); err != nil {
		t.Fatalf("ValidateRequest() error = %v", err)
	}

	if err := ValidateResult(request, result); err != nil {
		t.Fatalf("ValidateResult() error = %v", err)
	}

	first, err := MarshalResult(result)
	if err != nil {
		t.Fatalf("MarshalResult() error = %v", err)
	}

	second, err := MarshalResult(result)
	if err != nil {
		t.Fatalf("MarshalResult() second error = %v", err)
	}

	if string(first) != string(second) {
		t.Fatalf("MarshalResult() is not deterministic:\n%s\n%s", first, second)
	}
}

func TestValidateResultRejectsRequestDependentViolations(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		edit func(*Result)
		want string
	}{
		{name: "missing answer", edit: func(r *Result) { delete(r.Answers, "urgent") }, want: "answer IDs"},
		{name: "extra answer", edit: func(r *Result) { r.Answers["extra"] = r.Answers["urgent"] }, want: "answer IDs"},
		{name: "type mismatch", edit: func(r *Result) { a := r.Answers["urgent"]; a.Type = "choice"; r.Answers["urgent"] = a }, want: "want \"noul\""},
		{name: "unknown choice", edit: func(r *Result) { a := r.Answers["team"]; a.Choice = "legal"; r.Answers["team"] = a }, want: "unknown choice"},
		{name: "mismatched probability keys", edit: func(r *Result) {
			a := r.Answers["team"]
			delete(a.Probabilities, "technical")
			r.Answers["team"] = a
		}, want: "probability keys"},
		{name: "bad sum", edit: func(r *Result) { a := r.Answers["team"]; a.Probabilities["billing"] = 0.4; r.Answers["team"] = a }, want: "sum to one"},
		{name: "out of range probability", edit: func(r *Result) {
			a := r.Answers["team"]
			a.Probabilities["billing"] = 1.1
			a.Probabilities["technical"] = -0.1
			r.Answers["team"] = a
		}, want: "invalid probability"},
		{name: "nonfinite noul", edit: func(r *Result) {
			a := r.Answers["urgent"]
			value := math.NaN()
			a.Noul = &value
			r.Answers["urgent"] = a
		}, want: "finite"},
		{name: "nonfinite confidence", edit: func(r *Result) {
			a := r.Answers["team"]
			value := math.Inf(1)
			a.Confidence = &value
			r.Answers["team"] = a
		}, want: "finite"},
		{name: "bad legend", edit: func(r *Result) { a := r.Answers["risk"]; a.Legend["1"] = "Extreme"; r.Answers["risk"] = a }, want: "legend"},
		{name: "score outside rubric", edit: func(r *Result) { a := r.Answers["risk"]; value := 3.0; a.Score = &value; r.Answers["risk"] = a }, want: "outside"},
		{name: "bad weighted score", edit: func(r *Result) { a := r.Answers["risk"]; value := 2.0; a.Score = &value; r.Answers["risk"] = a }, want: "inconsistent"},
		{name: "noul confidence", edit: func(r *Result) {
			a := r.Answers["urgent"]
			value := 0.9
			a.Confidence = &value
			r.Answers["urgent"] = a
		}, want: "another answer type"},
		{name: "negative usage", edit: func(r *Result) { r.Usage.InputTokens = -1 }, want: "nonnegative"},
		{name: "unsafe provider metadata", edit: func(r *Result) { r.Provider = "bad\nprovider" }, want: "unsafe metadata"},
		{name: "invalid actual model", edit: func(r *Result) { r.Model = "bad model" }, want: "actual model"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := validResult()
			test.edit(&result)

			err := ValidateResult(validRequest(), result)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("ValidateResult() error = %v, want containing %q", err, test.want)
			}
		})
	}
}

func TestValidateRequestRejectsRenderedBlankInstruction(t *testing.T) {
	t.Parallel()

	request := validRequest()
	question := request.Questions["urgent"]
	question.Instructions = "  "

	request.Questions["urgent"] = question
	if err := ValidateRequest(request); err == nil || !strings.Contains(err.Error(), "must not be blank") {
		t.Fatalf("ValidateRequest() error = %v", err)
	}
}

func validRequest() Request {
	return Request{
		Model: "jev-1.13.0",
		State: map[string]any{"message": "Please help", "attempt": 2.0, "note": nil},
		Questions: map[string]agent.EvaluationQuestion{
			"urgent": {Type: "noul", Instructions: "Is it urgent?"},
			"team": {
				Type:         "choice",
				Instructions: "Which team?",
				Criteria:     map[string]any{"billing": "Payments", "technical": "Bugs"},
			},
			"risk": {
				Type:         "score",
				Instructions: "Rate risk",
				Criteria:     []any{"Low", "Medium", "High"},
			},
		},
	}
}

func validResult() Result {
	noul := 0.82
	confidence := 0.91
	scoreConfidence := 0.75
	score := 1.01
	cost := 0.000018

	return Result{
		Service:        "typesafe",
		RequestedModel: "jev-1.13.0",
		Model:          "jev-1.13.0",
		Answers: map[string]Answer{
			"urgent": {Type: "noul", Noul: &noul},
			"team": {
				Type:          "choice",
				Choice:        "billing",
				Probabilities: map[string]float64{"billing": 0.8, "technical": 0.2},
				Confidence:    &confidence,
			},
			"risk": {
				Type:          "score",
				Score:         &score,
				Legend:        map[string]any{"0": "Low", "1": "Medium", "2": "High"},
				Probabilities: map[string]float64{"0": 0.33, "1": 0.33, "2": 0.34},
				Confidence:    &scoreConfidence,
			},
		},
		Usage: &Usage{InputTokens: 100, OutputTokens: 0, Cost: &cost},
	}
}
