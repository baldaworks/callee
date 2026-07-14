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
	if path := strings.TrimSpace(r.Metadata.Path); path != "" {
		if _, err := exec.LookPath(path); err != nil {
			return nil, fmt.Errorf("role %q: executable %q was not found", r.ID, path)
		}
		resolved, err := agentconfig.NormalizeConfig(cfg, "")
		if err != nil {
			return nil, fmt.Errorf("normalize role %q runtime: %w", r.ID, err)
		}
		ag, err := acpagent.New(acpagent.Config{
			Context: context.Background(), Name: r.ID, Description: r.Metadata.Description,
			Command: append([]string{path}, r.Metadata.ExtraArgs...), WorkingDir: cwd,
			ReasoningEffort: resolved.ReasoningEffort, Stderr: os.Stderr,
			SessionConfig: acpSessionConfig(resolved.Model, resolved.Mode),
		})
		if err != nil {
			return nil, fmt.Errorf("start role %q runtime: %w", r.ID, err)
		}
		return newNormaConversation(ag, ag)
	}
	factory := agentfactory.New(map[string]agentconfig.Config{r.ID: cfg}, mcpregistry.New(nil), agentfactory.WithStderrWriter(os.Stderr))
	ag, err := factory.Build(context.Background(), agentfactory.BuildRequest{AgentID: r.ID, Name: r.ID, Description: r.Metadata.Description, WorkingDirectory: cwd})
	if err != nil {
		return nil, fmt.Errorf("start role %q runtime: %w", r.ID, err)
	}
	return newNormaConversation(ag, nil)
}

func acpSessionConfig(model, mode string) []acpagent.SessionConfigValue {
	var values []acpagent.SessionConfigValue
	if model != "" {
		values = append(values, acpagent.SessionConfigValue{ID: "model", Value: model})
	}
	if mode != "" {
		values = append(values, acpagent.SessionConfigValue{ID: "mode", Value: mode})
	}
	return values
}

type normaConversation struct {
	runner   *runner.Runner
	sessions session.Service
	closer   interface{ Close() error }
}

func newNormaConversation(ag agent.Agent, closer interface{ Close() error }) (Conversation, error) {
	service := session.InMemoryService()
	r, err := runner.New(runner.Config{AppName: "callee", Agent: ag, SessionService: service})
	if err != nil {
		return nil, err
	}
	return &normaConversation{runner: r, sessions: service, closer: closer}, nil
}

func (n *normaConversation) Start(ctx context.Context, prompt string) (string, string, error) {
	created, err := n.sessions.Create(ctx, &session.CreateRequest{AppName: "callee", UserID: "callee"})
	if err != nil {
		return "", "", err
	}
	id := created.Session.ID()
	content, err := n.turn(ctx, id, prompt)
	return id, content, err
}

func (n *normaConversation) Reply(ctx context.Context, threadID, prompt string) (string, error) {
	return n.turn(ctx, threadID, prompt)
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
