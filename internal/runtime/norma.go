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
	"github.com/rs/zerolog/log"
	"google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/runner"
	"google.golang.org/adk/v2/session"
	"google.golang.org/genai"
)

// NormaFactory builds ACP agents through Norma Runtime.
type NormaFactory struct{}

var buildNormaAgent = buildNormaAgentDefault

const sessionConfigCapacity = 2

func (NormaFactory) New(provider Provider) (Conversation, error) {
	ag, closer, err := buildNormaAgent(context.Background(), provider)
	if err != nil {
		return nil, err
	}

	conversation, err := newNormaConversation(ag, closer)
	if err != nil {
		if closer != nil {
			_ = closer.Close()
		}

		return nil, err
	}

	return conversation, nil
}

// Check starts and initializes a role's ACP runtime, then closes it without
// creating a session or sending a model prompt.
func (NormaFactory) Check(ctx context.Context, r role.Role) error {
	provider, err := ProviderFor(r)
	if err != nil {
		return err
	}

	_, closer, err := buildNormaAgent(ctx, provider)
	if err != nil {
		return err
	}

	if closer == nil {
		return nil
	}

	if err := closer.Close(); err != nil {
		return fmt.Errorf("close role %q runtime: %w", r.ID, err)
	}

	return nil
}

func buildNormaAgentDefault(ctx context.Context, provider Provider) (agent.Agent, interface{ Close() error }, error) {
	log.Debug().Str("provider", provider.Type()).Msg("normalizing provider runtime configuration")

	cwd, err := os.Getwd()
	if err != nil {
		return nil, nil, fmt.Errorf("get working directory: %w", err)
	}

	if len(provider.command) > 0 {
		command := provider.command[0]
		if _, err := exec.LookPath(command); err != nil {
			return nil, nil, fmt.Errorf("provider %q: executable %q was not found", provider.Type(), command)
		}
	}

	factory := agentfactory.New(map[string]agentconfig.Config{provider.Key(): provider.config}, mcpregistry.New(nil), agentfactory.WithStderrWriter(os.Stderr))
	log.Debug().Str("provider", provider.Type()).Msg("initializing ACP runtime")

	ag, err := factory.Build(ctx, agentfactory.BuildRequest{AgentID: provider.Key(), Name: provider.Type(), Description: provider.Type(), WorkingDirectory: cwd})
	if err != nil {
		return nil, nil, fmt.Errorf("start provider %q runtime: %w", provider.Type(), err)
	}

	closer, _ := ag.(interface{ Close() error })

	log.Debug().Str("provider", provider.Type()).Msg("ACP runtime initialized")

	return ag, closer, nil
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

func (n *normaConversation) Start(ctx context.Context, r role.Role, prompt string) (string, string, error) {
	created, err := n.sessions.Create(ctx, &session.CreateRequest{AppName: "callee", UserID: "callee", State: roleSessionState(r)})
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

func roleSessionState(r role.Role) map[string]any {
	configValues := make([]acpagent.SessionConfigValue, 0, sessionConfigCapacity)
	if model := strings.TrimSpace(r.Metadata.Model); model != "" {
		configValues = append(configValues, acpagent.SelectSessionConfigValue("model", model))
	}

	if mode := strings.TrimSpace(r.Metadata.Mode); mode != "" {
		configValues = append(configValues, acpagent.SelectSessionConfigValue("mode", mode))
	}

	acpState := make(map[string]any)
	if len(configValues) > 0 {
		acpState["config_values"] = configValues
	}

	if reasoning := strings.TrimSpace(r.Metadata.Reasoning); reasoning != "" {
		acpState["meta"] = map[string]any{
			"codex": map[string]any{"config": map[string]any{"model_reasoning_effort": reasoning}},
		}
	}

	if len(acpState) == 0 {
		return nil
	}

	return map[string]any{acpagent.SessionStateKey: acpState}
}

func (n *normaConversation) Reply(ctx context.Context, threadID, prompt string) (string, error) {
	localID, ok := n.threads[threadID]
	if !ok {
		return "", fmt.Errorf("thread %q is not available", threadID)
	}

	return n.turn(ctx, localID, prompt)
}

func (n *normaConversation) Close() error {
	if n.closer != nil {
		return n.closer.Close()
	}

	return nil
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
