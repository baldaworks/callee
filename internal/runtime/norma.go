package runtime

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/baldaworks/callee/internal/role"
	acpagent "github.com/normahq/go-adk-acpagent/v2"
	"github.com/normahq/runtime/v2/agentconfig"
	"github.com/normahq/runtime/v2/agentfactory"
	"github.com/normahq/runtime/v2/mcpregistry"
	"google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/runner"
	"google.golang.org/adk/v2/session"
	"google.golang.org/genai"
)

// NormaFactory builds ACP agents through Norma Runtime.
type NormaFactory struct{}

func (NormaFactory) New(r role.Role) (Conversation, error) {
	cfg, err := Normalize(r)
	if err != nil {
		return nil, err
	}
	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("get working directory: %w", err)
	}
	if command := strings.TrimSpace(r.Metadata.Cmd); command != "" {
		if _, err := exec.LookPath(command); err != nil {
			return nil, fmt.Errorf("role %q: executable %q was not found", r.ID, command)
		}
	}
	factory := agentfactory.New(map[string]agentconfig.Config{r.ID: cfg}, mcpregistry.New(nil), agentfactory.WithStderrWriter(os.Stderr))
	ag, err := factory.Build(context.Background(), agentfactory.BuildRequest{AgentID: r.ID, Name: r.ID, Description: r.Metadata.Description, WorkingDirectory: cwd})
	if err != nil {
		return nil, fmt.Errorf("start role %q runtime: %w", r.ID, err)
	}
	closer, _ := ag.(interface{ Close() error })
	return newNormaConversation(ag, closer)
}

type normaConversation struct {
	runner   *runner.Runner
	sessions session.Service
	closer   interface{ Close() error }
	threads  map[string]string
}

func newNormaConversation(ag agent.Agent, closer interface{ Close() error }) (Conversation, error) {
	service := session.InMemoryService()
	r, err := runner.New(runner.Config{AppName: "callee", Agent: ag, SessionService: service})
	if err != nil {
		return nil, err
	}
	return &normaConversation{runner: r, sessions: service, closer: closer, threads: map[string]string{}}, nil
}

func (n *normaConversation) Start(ctx context.Context, prompt string) (string, string, error) {
	created, err := n.sessions.Create(ctx, &session.CreateRequest{AppName: "callee", UserID: "callee"})
	if err != nil {
		return "", "", err
	}
	localID := created.Session.ID()
	content, err := n.turn(ctx, localID, prompt)
	if err != nil {
		return "", "", err
	}
	threadID, err := n.acpThreadID(ctx, localID)
	if err != nil {
		return "", "", err
	}
	n.threads[threadID] = localID
	return threadID, content, nil
}

func (n *normaConversation) Reply(ctx context.Context, threadID, prompt string) (string, error) {
	localID, ok := n.threads[threadID]
	if !ok {
		return "", fmt.Errorf("thread %q is not available", threadID)
	}
	return n.turn(ctx, localID, prompt)
}

func (n *normaConversation) acpThreadID(ctx context.Context, localID string) (string, error) {
	result, err := n.sessions.Get(ctx, &session.GetRequest{AppName: "callee", UserID: "callee", SessionID: localID})
	if err != nil {
		return "", fmt.Errorf("get ACP session state: %w", err)
	}
	state, err := result.Session.State().Get(acpagent.SessionStateKey)
	if err != nil {
		return "", fmt.Errorf("read ACP session state: %w", err)
	}
	values, ok := state.(map[string]any)
	if !ok {
		return "", fmt.Errorf("read ACP session state: invalid value")
	}
	threadID, _ := values["session_id"].(string)
	if threadID == "" {
		return "", fmt.Errorf("read ACP session state: missing session_id")
	}
	return threadID, nil
}
func (n *normaConversation) Close() error {
	if n.closer != nil {
		return n.closer.Close()
	}
	return nil
}

func (n *normaConversation) turn(ctx context.Context, id, prompt string) (string, error) {
	var final string
	for event, err := range n.runner.Run(ctx, "callee", id, genai.NewContentFromText(prompt, genai.RoleUser), agent.RunConfig{}) {
		if err != nil {
			return "", err
		}
		if event.IsFinalResponse() && event.Content != nil {
			for _, part := range event.Content.Parts {
				final += part.Text
			}
		}
	}
	if final == "" {
		return "", fmt.Errorf("runtime returned no final text")
	}
	return final, nil
}
