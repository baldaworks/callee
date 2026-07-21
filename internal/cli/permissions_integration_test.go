package cli

import (
	"bytes"
	"context"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	resource "github.com/baldaworks/callee/internal/agent"
	"github.com/baldaworks/callee/internal/runtime"
	acp "github.com/coder/acp-go-sdk"
)

const permissionACPHelper = "CALLEE_PERMISSION_ACP_HELPER"

func TestWorkflowPermissionRequestCrossesACPWire(t *testing.T) {
	if os.Getenv(permissionACPHelper) == "1" {
		runPermissionACPHelper()

		return
	}

	tests := []struct {
		name string
		mode resource.PermissionMode
		want string
	}{
		{name: "allow", mode: resource.PermissionModeAllow, want: "allow"},
		{name: "deny", mode: resource.PermissionModeDeny, want: "deny"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv(permissionACPHelper, "1")

			role := permissionWireRole(test.mode)

			provider, err := runtime.ProviderForAgent(role)
			if err != nil {
				t.Fatalf("ProviderForAgent() error: %v", err)
			}

			var stderr synchronizedBuffer

			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()

			process, err := (workflowProcessFactory{stderr: &stderr}).Start(ctx, provider)
			if err != nil {
				t.Fatalf("Start() error: %v; stderr=%q", err, stderr.String())
			}
			defer func() {
				if err := process.Close(); err != nil && !strings.Contains(err.Error(), "context canceled") {
					t.Errorf("Close() error: %v; stderr=%q", err, stderr.String())
				}
			}()

			session, err := process.NewSession(ctx, role, "wire_worker")
			if err != nil {
				t.Fatalf("NewSession() error: %v", err)
			}

			if err := session.Prepare(ctx); err != nil {
				t.Fatalf("Prepare() error: %v; stderr=%q", err, stderr.String())
			}

			output, err := session.Turn(ctx, "request permission")
			if err != nil {
				t.Fatalf("Turn() error: %v; stderr=%q", err, stderr.String())
			}

			if output.Content != test.want {
				t.Errorf("Turn() output = %q, want %q", output.Content, test.want)
			}
		})
	}
}

type synchronizedBuffer struct {
	mu     sync.Mutex
	buffer bytes.Buffer
}

func (b *synchronizedBuffer) Write(data []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.buffer.Write(data)
}

func (b *synchronizedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.buffer.String()
}

func permissionWireRole(mode resource.PermissionMode) resource.Resource {
	return resource.Resource{
		ID:   "roles/wire-worker",
		Kind: resource.RoleKind,
		Spec: resource.Spec{
			Provider: &resource.Provider{
				Type:      "codex",
				Cmd:       os.Args[0],
				ExtraArgs: []string{"-test.run=^TestWorkflowPermissionRequestCrossesACPWire$"},
			},
			Permissions: &resource.Permissions{Mode: mode},
		},
	}
}

type permissionWireAgent struct {
	connection *acp.AgentSideConnection
}

func runPermissionACPHelper() {
	agent := &permissionWireAgent{}
	connection := acp.NewAgentSideConnection(agent, os.Stdout, os.Stdin)
	agent.connection = connection

	<-connection.Done()
	os.Exit(0)
}

func (*permissionWireAgent) Authenticate(context.Context, acp.AuthenticateRequest) (acp.AuthenticateResponse, error) {
	return acp.AuthenticateResponse{}, nil
}

func (*permissionWireAgent) Initialize(context.Context, acp.InitializeRequest) (acp.InitializeResponse, error) {
	return acp.InitializeResponse{
		ProtocolVersion:   acp.ProtocolVersionNumber,
		AgentCapabilities: acp.AgentCapabilities{},
	}, nil
}

func (*permissionWireAgent) Logout(context.Context, acp.LogoutRequest) (acp.LogoutResponse, error) {
	return acp.LogoutResponse{}, nil
}

func (*permissionWireAgent) Cancel(context.Context, acp.CancelNotification) error { return nil }

func (*permissionWireAgent) CloseSession(context.Context, acp.CloseSessionRequest) (acp.CloseSessionResponse, error) {
	return acp.CloseSessionResponse{}, nil
}

func (*permissionWireAgent) ListSessions(context.Context, acp.ListSessionsRequest) (acp.ListSessionsResponse, error) {
	return acp.ListSessionsResponse{}, nil
}

func (*permissionWireAgent) NewSession(context.Context, acp.NewSessionRequest) (acp.NewSessionResponse, error) {
	return acp.NewSessionResponse{SessionId: "permission-wire-session"}, nil
}

func (a *permissionWireAgent) Prompt(ctx context.Context, request acp.PromptRequest) (acp.PromptResponse, error) {
	response, err := a.connection.RequestPermission(ctx, acp.RequestPermissionRequest{
		SessionId: request.SessionId,
		ToolCall: acp.ToolCallUpdate{
			ToolCallId: "permission-wire-tool",
			Title:      acp.Ptr("Wire permission request"),
			Kind:       acp.Ptr(acp.ToolKindExecute),
			Content: []acp.ToolCallContent{
				acp.ToolContent(acp.TextBlock("Run the wire test command.")),
			},
			RawInput: "wire secret",
		},
		Options: []acp.PermissionOption{
			{Name: "Allow once", Kind: acp.PermissionOptionKindAllowOnce, OptionId: "allow"},
			{Name: "Reject once", Kind: acp.PermissionOptionKindRejectOnce, OptionId: "deny"},
		},
	})
	if err != nil {
		return acp.PromptResponse{}, err
	}

	answer := "cancelled"
	if response.Outcome.Selected != nil {
		answer = string(response.Outcome.Selected.OptionId)
	}

	if err := a.connection.SessionUpdate(ctx, acp.SessionNotification{
		SessionId: request.SessionId,
		Update:    acp.UpdateAgentMessageText(answer),
	}); err != nil {
		return acp.PromptResponse{}, err
	}

	return acp.PromptResponse{StopReason: acp.StopReasonEndTurn}, nil
}

func (*permissionWireAgent) ResumeSession(context.Context, acp.ResumeSessionRequest) (acp.ResumeSessionResponse, error) {
	return acp.ResumeSessionResponse{}, nil
}

func (*permissionWireAgent) SetSessionConfigOption(
	context.Context,
	acp.SetSessionConfigOptionRequest,
) (acp.SetSessionConfigOptionResponse, error) {
	return acp.SetSessionConfigOptionResponse{}, nil
}

func (*permissionWireAgent) SetSessionMode(context.Context, acp.SetSessionModeRequest) (acp.SetSessionModeResponse, error) {
	return acp.SetSessionModeResponse{}, nil
}
