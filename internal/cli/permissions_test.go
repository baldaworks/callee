package cli

import (
	"bufio"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/baldaworks/callee/internal/agent"
	acp "github.com/coder/acp-go-sdk"
)

func TestWorkflowPermissionControllerAutomaticPolicies(t *testing.T) {
	t.Parallel()

	mixedOptions := []acp.PermissionOption{
		{Name: "always", Kind: acp.PermissionOptionKindAllowAlways, OptionId: "allow-always"},
		{Name: "reject always", Kind: acp.PermissionOptionKindRejectAlways, OptionId: "reject-always"},
		{Name: "once", Kind: acp.PermissionOptionKindAllowOnce, OptionId: "allow-once-first"},
		{Name: "once duplicate", Kind: acp.PermissionOptionKindAllowOnce, OptionId: "allow-once-second"},
		{Name: "reject once", Kind: acp.PermissionOptionKindRejectOnce, OptionId: "reject-once"},
	}
	tests := []struct {
		name    string
		mode    agent.PermissionMode
		options []acp.PermissionOption
		want    acp.PermissionOptionId
	}{
		{name: "allow prefers first allow once", mode: agent.PermissionModeAllow, options: mixedOptions, want: "allow-once-first"},
		{name: "allow falls back to allow always", mode: agent.PermissionModeAllow, options: []acp.PermissionOption{{Name: "always", Kind: acp.PermissionOptionKindAllowAlways, OptionId: "allow-always"}}, want: "allow-always"},
		{name: "deny prefers first reject once", mode: agent.PermissionModeDeny, options: mixedOptions, want: "reject-once"},
		{name: "deny falls back to reject always", mode: agent.PermissionModeDeny, options: []acp.PermissionOption{{Name: "always", Kind: acp.PermissionOptionKindRejectAlways, OptionId: "reject-always"}}, want: "reject-always"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			pauses := &permissionPauserStub{}
			controller := newWorkflowPermissionController(nil, nil)
			controller.pauses = pauses
			controller.Bind("session", permissionRole("worker", test.mode))

			response, err := controller.Handle(context.Background(), acp.RequestPermissionRequest{
				SessionId: "session",
				Options:   test.options,
			})
			if err != nil {
				t.Fatalf("Handle() error: %v", err)
			}

			if response.Outcome.Selected == nil || response.Outcome.Selected.OptionId != test.want {
				t.Errorf("Handle() outcome = %+v, want option %q", response.Outcome, test.want)
			}

			if pauses.pauses != 0 || pauses.resumes != 0 {
				t.Errorf("automatic policy paused timeout: %d/%d", pauses.pauses, pauses.resumes)
			}
		})
	}
}

func TestWorkflowPermissionControllerKeepsPoliciesPerSession(t *testing.T) {
	t.Parallel()

	controller := newWorkflowPermissionController(nil, nil)
	controller.Bind("allow-session", permissionRole("drafter_step1", agent.PermissionModeAllow))
	controller.Bind("deny-session", permissionRole("drafter_step2", agent.PermissionModeDeny))

	options := []acp.PermissionOption{
		{Name: "allow", Kind: acp.PermissionOptionKindAllowOnce, OptionId: "allow"},
		{Name: "deny", Kind: acp.PermissionOptionKindRejectOnce, OptionId: "deny"},
	}

	for sessionID, want := range map[acp.SessionId]acp.PermissionOptionId{
		"allow-session": "allow",
		"deny-session":  "deny",
	} {
		response, err := controller.Handle(context.Background(), acp.RequestPermissionRequest{SessionId: sessionID, Options: options})
		if err != nil {
			t.Fatalf("Handle(%q) error: %v", sessionID, err)
		}

		if response.Outcome.Selected == nil || response.Outcome.Selected.OptionId != want {
			t.Errorf("Handle(%q) outcome = %+v, want option %q", sessionID, response.Outcome, want)
		}
	}
}

func TestWorkflowPermissionControllerAskUsesTTYAndPauses(t *testing.T) {
	t.Parallel()

	terminal := &splitTerminal{input: strings.NewReader("2\n")}
	pauses := &permissionPauserStub{}
	controller := newWorkflowPermissionController(&terminalInteractor{
		reader:   bufio.NewReader(terminal),
		terminal: terminal,
		timeout:  time.Second,
	}, nil)
	controller.pauses = pauses
	controller.Bind("session", agent.Resource{ID: "worker"})

	response, err := controller.Handle(context.Background(), acp.RequestPermissionRequest{
		SessionId: "session",
		Options: []acp.PermissionOption{
			{Name: "allow once", Kind: acp.PermissionOptionKindAllowOnce, OptionId: "opaque-first"},
			{Name: "reject once", Kind: acp.PermissionOptionKindRejectOnce, OptionId: "opaque-second"},
		},
	})
	if err != nil {
		t.Fatalf("Handle() error: %v", err)
	}

	if response.Outcome.Selected == nil || response.Outcome.Selected.OptionId != "opaque-second" {
		t.Errorf("Handle() outcome = %+v, want exact second option ID", response.Outcome)
	}

	if pauses.pauses != 1 || pauses.resumes != 1 {
		t.Errorf("pause calls = %d/%d, want 1/1", pauses.pauses, pauses.resumes)
	}

	for _, want := range []string{"Permission required:", "1) allow once [allow_once]", "2) reject once [reject_once]", "Select: "} {
		if !strings.Contains(terminal.output.String(), want) {
			t.Errorf("terminal output = %q, want containing %q", terminal.output.String(), want)
		}
	}
}

func TestWorkflowPermissionControllerCancelsEmptyAndInvalidAsk(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		options []acp.PermissionOption
		paused  bool
	}{
		{name: "empty options", options: nil},
		{name: "invalid selection", input: "9\n", options: []acp.PermissionOption{{Name: "allow", Kind: acp.PermissionOptionKindAllowOnce, OptionId: "allow"}}, paused: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			terminal := &splitTerminal{input: strings.NewReader(test.input)}
			pauses := &permissionPauserStub{}
			controller := newWorkflowPermissionController(&terminalInteractor{
				reader:   bufio.NewReader(terminal),
				terminal: terminal,
				timeout:  time.Second,
			}, nil)
			controller.pauses = pauses
			controller.Bind("session", permissionRole("worker", agent.PermissionModeAsk))

			response, err := controller.Handle(context.Background(), acp.RequestPermissionRequest{SessionId: "session", Options: test.options})
			if err != nil {
				t.Fatalf("Handle() error: %v", err)
			}

			if response.Outcome.Cancelled == nil {
				t.Errorf("Handle() outcome = %+v, want cancelled", response.Outcome)
			}

			wantCalls := 0
			if test.paused {
				wantCalls = 1
			}

			if pauses.pauses != wantCalls || pauses.resumes != wantCalls {
				t.Errorf("pause calls = %d/%d, want %d/%d", pauses.pauses, pauses.resumes, wantCalls, wantCalls)
			}
		})
	}
}

func TestWorkflowPermissionControllerRejectsIncompatibleAndUnboundRequests(t *testing.T) {
	t.Parallel()

	controller := newWorkflowPermissionController(nil, nil)
	controller.Bind("session", permissionRole("validator", agent.PermissionModeAllow))

	for _, options := range [][]acp.PermissionOption{
		nil,
		{{Name: "reject", Kind: acp.PermissionOptionKindRejectOnce, OptionId: "reject"}},
		{{Name: "custom", Kind: acp.PermissionOptionKind("custom_kind"), OptionId: "custom"}},
	} {
		_, err := controller.Handle(context.Background(), acp.RequestPermissionRequest{
			SessionId: "session",
			Options:   options,
			ToolCall:  acp.ToolCallUpdate{RawInput: "secret arguments"},
		})
		if err == nil {
			t.Fatal("Handle() error = nil, want incompatible options error")
		}

		for _, want := range []string{`agent "validator"`, `policy "allow"`, "offered kinds:"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("Handle() error = %v, want containing %q", err, want)
			}
		}

		if strings.Contains(err.Error(), "secret arguments") {
			t.Errorf("Handle() error leaked tool arguments: %v", err)
		}
	}

	if _, err := controller.Handle(context.Background(), acp.RequestPermissionRequest{SessionId: "missing"}); err == nil || !strings.Contains(err.Error(), "unbound ACP session") {
		t.Fatalf("Handle(unbound) error = %v", err)
	}
}

type permissionPauserStub struct {
	pauses  int
	resumes int
}

func (p *permissionPauserStub) Pause(context.Context) error {
	p.pauses++

	return nil
}

func (p *permissionPauserStub) Resume(context.Context) error {
	p.resumes++

	return nil
}

func permissionRole(id string, mode agent.PermissionMode) agent.Resource {
	return agent.Resource{
		ID: id,
		Spec: agent.Spec{
			Permissions: &agent.Permissions{Mode: mode},
		},
	}
}
