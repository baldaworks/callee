package cli

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/baldaworks/callee/internal/agent"
	acp "github.com/coder/acp-go-sdk"
	"github.com/rs/zerolog"
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
		ToolCall: acp.ToolCallUpdate{
			ToolCallId: "tool-call",
			Title:      acp.Ptr("Exec command approval"),
			Kind:       acp.Ptr(acp.ToolKindExecute),
			Content: []acp.ToolCallContent{
				acp.ToolContent(acp.TextBlock("The command needs network access.\n\nCommand:\n```sh\ncurl example.com\n```\n\nWorking directory: `/tmp/work`")),
			},
			RawInput: "raw input must not be displayed",
		},
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

	for _, want := range []string{
		"Permission required:",
		"Exec command approval [execute]",
		"The command needs network access.",
		"curl example.com",
		"Working directory: `/tmp/work`",
		"1) allow once [allow_once]",
		"2) reject once [reject_once]",
		"Select: ",
	} {
		if !strings.Contains(terminal.output.String(), want) {
			t.Errorf("terminal output = %q, want containing %q", terminal.output.String(), want)
		}
	}

	if strings.Contains(terminal.output.String(), "raw input must not be displayed") {
		t.Errorf("terminal output leaked raw input: %q", terminal.output.String())
	}
}

func TestWorkflowPermissionControllerLogsAutomaticAnswers(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		mode     agent.PermissionMode
		wantKind acp.PermissionOptionKind
	}{
		{name: "allow", mode: agent.PermissionModeAllow, wantKind: acp.PermissionOptionKindAllowOnce},
		{name: "deny", mode: agent.PermissionModeDeny, wantKind: acp.PermissionOptionKindRejectOnce},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			var output bytes.Buffer

			logger := zerolog.New(&output)
			ctx := logger.WithContext(context.Background())

			controller := newWorkflowPermissionController(nil, nil)
			controller.Bind("session", permissionRole("worker", test.mode))

			response, err := controller.Handle(ctx, permissionLogRequest())
			if err != nil {
				t.Fatalf("Handle() error: %v", err)
			}

			if response.Outcome.Selected == nil {
				t.Fatalf("Handle() outcome = %+v, want selected", response.Outcome)
			}

			events := decodePermissionLogEvents(t, output.String())
			if len(events) != 2 {
				t.Fatalf("log events = %#v, want received and answered", events)
			}

			assertPermissionLogFields(t, events[0], "permission request received", test.mode)
			assertPermissionLogFields(t, events[1], "permission request answered", test.mode)

			if got := events[1]["outcome"]; got != "selected" {
				t.Errorf("answered outcome = %#v, want selected", got)
			}

			if got := events[1]["option_kind"]; got != string(test.wantKind) {
				t.Errorf("answered option_kind = %#v, want %q", got, test.wantKind)
			}

			if _, ok := events[1]["duration"]; !ok {
				t.Errorf("answered event has no duration: %#v", events[1])
			}

			for _, secret := range []string{"private command content", "raw secret arguments", "opaque-allow"} {
				if strings.Contains(output.String(), secret) {
					t.Errorf("permission logs leaked %q: %s", secret, output.String())
				}
			}
		})
	}
}

func TestWorkflowPermissionControllerLogsCancellation(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer

	logger := zerolog.New(&output)
	ctx := logger.WithContext(context.Background())

	controller := newWorkflowPermissionController(nil, nil)
	controller.Bind("session", permissionRole("worker", agent.PermissionModeAsk))

	request := permissionLogRequest()
	request.Options = nil

	response, err := controller.Handle(ctx, request)
	if err != nil {
		t.Fatalf("Handle() error: %v", err)
	}

	if response.Outcome.Cancelled == nil {
		t.Fatalf("Handle() outcome = %+v, want cancelled", response.Outcome)
	}

	events := decodePermissionLogEvents(t, output.String())
	if len(events) != 2 || events[1]["message"] != "permission request answered" || events[1]["outcome"] != "cancelled" {
		t.Fatalf("log events = %#v, want received and cancelled answer", events)
	}

	if _, ok := events[1]["option_kind"]; ok {
		t.Errorf("cancelled event has option_kind: %#v", events[1])
	}
}

func TestWorkflowPermissionControllerLogsFailures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		bind bool
	}{
		{name: "incompatible options", bind: true},
		{name: "unbound session"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			var output bytes.Buffer

			logger := zerolog.New(&output)
			ctx := logger.WithContext(context.Background())

			controller := newWorkflowPermissionController(nil, nil)
			if test.bind {
				controller.Bind("session", permissionRole("validator", agent.PermissionModeAllow))
			}

			request := permissionLogRequest()
			request.Options = []acp.PermissionOption{{Name: "deny", Kind: acp.PermissionOptionKindRejectOnce, OptionId: "opaque-deny"}}

			if _, err := controller.Handle(ctx, request); err == nil {
				t.Fatal("Handle() error = nil, want failure")
			}

			events := decodePermissionLogEvents(t, output.String())
			if len(events) != 2 || events[0]["message"] != "permission request received" || events[1]["message"] != "permission request failed" {
				t.Fatalf("log events = %#v, want received and failed", events)
			}

			if got := events[1]["level"]; got != "error" {
				t.Errorf("failed level = %#v, want error", got)
			}

			if _, ok := events[1]["duration"]; !ok {
				t.Errorf("failed event has no duration: %#v", events[1])
			}

			for _, secret := range []string{"private command content", "raw secret arguments", "opaque-deny"} {
				if strings.Contains(output.String(), secret) {
					t.Errorf("permission logs leaked %q: %s", secret, output.String())
				}
			}
		})
	}
}

func TestPermissionRequestContentDescribesNonTextBlocks(t *testing.T) {
	t.Parallel()

	contents := []acp.ToolCallContent{
		acp.ToolContent(acp.ImageBlock("encoded", "image/png")),
		acp.ToolContent(acp.AudioBlock("encoded", "audio/wav")),
		acp.ToolContent(acp.ResourceLinkBlock("design", "file:///tmp/design.md")),
		acp.ToolDiffContent("README.md", "new", "old"),
		acp.ToolTerminalRef("terminal-1"),
		{},
	}

	got := permissionRequestContent(contents)
	for _, want := range []string{
		"[image: image/png]",
		"[audio: audio/wav]",
		"[resource: design <file:///tmp/design.md>]",
		"[file diff: README.md]",
		"[terminal: terminal-1]",
		"[unsupported content]",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("permissionRequestContent() = %q, want containing %q", got, want)
		}
	}

	if strings.Contains(got, "encoded") || strings.Contains(got, "new") || strings.Contains(got, "old") {
		t.Errorf("permissionRequestContent() exposed non-text payload: %q", got)
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

func permissionLogRequest() acp.RequestPermissionRequest {
	return acp.RequestPermissionRequest{
		SessionId: "session",
		ToolCall: acp.ToolCallUpdate{
			ToolCallId: "tool-call",
			Title:      acp.Ptr("Command approval"),
			Kind:       acp.Ptr(acp.ToolKindExecute),
			Content:    []acp.ToolCallContent{acp.ToolContent(acp.TextBlock("private command content"))},
			RawInput:   "raw secret arguments",
		},
		Options: []acp.PermissionOption{
			{Name: "allow", Kind: acp.PermissionOptionKindAllowOnce, OptionId: "opaque-allow"},
			{Name: "deny", Kind: acp.PermissionOptionKindRejectOnce, OptionId: "opaque-deny"},
		},
	}
}

func decodePermissionLogEvents(t *testing.T, output string) []map[string]any {
	t.Helper()

	var events []map[string]any

	for line := range strings.SplitSeq(strings.TrimSpace(output), "\n") {
		var event map[string]any

		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatalf("decode log event %q: %v", line, err)
		}

		events = append(events, event)
	}

	return events
}

func assertPermissionLogFields(t *testing.T, event map[string]any, message string, mode agent.PermissionMode) {
	t.Helper()

	want := map[string]any{
		"message":        message,
		"id":             "worker",
		"kind":           "Role",
		"policy":         string(mode),
		"title":          "Command approval",
		"tool_kind":      "execute",
		"acp_session_id": "session",
		"tool_call_id":   "tool-call",
	}
	for key, value := range want {
		if got := event[key]; got != value {
			t.Errorf("%s = %#v, want %#v in event %#v", key, got, value, event)
		}
	}
}
