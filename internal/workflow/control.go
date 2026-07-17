// Package workflow executes resolved Callee agent trees.
package workflow

import (
	"fmt"
	"strings"
)

const controlPrefix = "callee.control."

const (
	controlAwait    = "callee.control.v1.await"
	controlReturn   = "callee.control.v1.return"
	controlEscalate = "callee.control.v1.escalate"
	controlFail     = "callee.control.v1.fail"
)

type outcome int

const (
	outcomeReturn outcome = iota
	outcomeAwait
	outcomeEscalate
	outcomeFail
)

func (o outcome) String() string {
	switch o {
	case outcomeReturn:
		return "return"
	case outcomeAwait:
		return "await"
	case outcomeEscalate:
		return "escalate"
	case outcomeFail:
		return "fail"
	default:
		return "unknown"
	}
}

type response struct {
	outcome  outcome
	artifact string
}

func parseResponse(content string, repl bool) (response, error) {
	logical := strings.TrimSuffix(content, "\n")
	logical = strings.TrimSuffix(logical, "\r")

	lineStart := strings.LastIndexByte(logical, '\n') + 1
	control := strings.TrimSuffix(logical[lineStart:], "\r")

	parsedOutcome, hasControl := controlOutcome(control)
	if !hasControl {
		if strings.HasPrefix(control, controlPrefix) {
			return response{}, fmt.Errorf("malformed workflow control record %q", control)
		}

		if repl {
			return response{}, fmt.Errorf("REPL response is missing a workflow control record")
		}

		if strings.TrimSpace(content) == "" {
			return response{}, fmt.Errorf("runtime returned an empty artifact")
		}

		return response{outcome: outcomeReturn, artifact: content}, nil
	}

	if parsedOutcome == outcomeAwait && !repl {
		return response{}, fmt.Errorf("non-REPL response cannot use %q", controlAwait)
	}

	prefix := logical[:lineStart]

	artifact, err := controlArtifact(prefix)
	if err != nil {
		return response{}, err
	}

	switch parsedOutcome {
	case outcomeAwait, outcomeReturn:
		if strings.TrimSpace(artifact) == "" {
			return response{}, fmt.Errorf("workflow control record %q requires a nonempty artifact", control)
		}
	case outcomeEscalate, outcomeFail:
		// Both records may be control-only. Failure text is diagnostic detail.
	}

	return response{outcome: parsedOutcome, artifact: artifact}, nil
}

func controlOutcome(line string) (outcome, bool) {
	switch line {
	case controlAwait:
		return outcomeAwait, true
	case controlReturn:
		return outcomeReturn, true
	case controlEscalate:
		return outcomeEscalate, true
	case controlFail:
		return outcomeFail, true
	default:
		return 0, false
	}
}

func controlArtifact(prefix string) (string, error) {
	if prefix == "" {
		return "", nil
	}

	switch {
	case strings.HasSuffix(prefix, "\r\n\r\n"):
		artifact := strings.TrimSuffix(prefix, "\r\n\r\n")
		if strings.HasSuffix(artifact, "\n") || strings.HasSuffix(artifact, "\r") {
			return "", fmt.Errorf("workflow control record must be separated from its artifact by exactly one empty line")
		}

		return artifact, nil
	case strings.HasSuffix(prefix, "\n\n"):
		artifact := strings.TrimSuffix(prefix, "\n\n")
		if strings.HasSuffix(artifact, "\n") || strings.HasSuffix(artifact, "\r") {
			return "", fmt.Errorf("workflow control record must be separated from its artifact by exactly one empty line")
		}

		return artifact, nil
	default:
		return "", fmt.Errorf("workflow control record must be separated from its artifact by exactly one empty line")
	}
}

func controlInstructions(repl, canEscalate bool) string {
	if repl {
		instructions := []string{
			"Finish every assistant response with exactly one control record on its own final line, separated from preceding text by one empty line:",
			"- callee.control.v1.await — ask the operator for another turn; preceding text is required.",
			"- callee.control.v1.return — finish this role successfully; preceding artifact is required.",
		}
		if canEscalate {
			instructions = append(instructions, "- callee.control.v1.escalate — return control to the nearest Loop; preceding artifact is optional.")
		}

		instructions = append(instructions, "- callee.control.v1.fail — fail the workflow; preceding diagnostic detail is optional.")

		return "\n\n---\nCallee workflow control protocol:\n" + strings.Join(instructions, "\n")
	}

	instructions := []string{"Return the requested artifact normally."}
	if canEscalate {
		instructions = append(instructions, "To return control to the nearest Loop, finish with callee.control.v1.escalate on its own final line.")
	}

	instructions = append(instructions,
		"To fail the workflow, finish with callee.control.v1.fail on its own final line.",
		"Separate a preceding artifact or diagnostic from the record by one empty line.",
	)

	return "\n\n---\nCallee workflow control protocol:\n" + strings.Join(instructions, "\n")
}

func replReminder() string {
	return "\n\nRemember: finish this response with exactly one callee.control.v1 control record."
}
