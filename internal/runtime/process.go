package runtime

import (
	"context"
	"errors"
	"fmt"
	"strings"

	resource "github.com/baldaworks/callee/internal/agent"
	acpagent "github.com/normahq/go-adk-acpagent/v2"
	"google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/runner"
	"google.golang.org/adk/v2/session"
	"google.golang.org/genai"
)

// ProcessFactory starts one reusable provider transport process.
type ProcessFactory interface {
	Start(ctx context.Context, provider Provider) (ProviderProcess, error)
}

// ProviderProcess creates fresh Role visit sessions on one provider process.
type ProviderProcess interface {
	NewSession(ctx context.Context, role resource.Resource) (AgentSession, error)
	Close() error
}

// AgentSession executes turns for one concrete Role visit.
type AgentSession interface {
	Prepare(ctx context.Context) error
	Turn(ctx context.Context, prompt string) (string, error)
}

// Start initializes one reusable Norma-backed provider process.
func (f NormaFactory) Start(ctx context.Context, provider Provider) (ProviderProcess, error) {
	stderr, flush := f.diagnosticsWriter()

	agentInstance, closer, err := buildNormaAgent(ctx, provider, stderr, f.PermissionHandler)
	if err != nil {
		if flushErr := flush(); flushErr != nil {
			return nil, errors.Join(err, flushErr)
		}

		return nil, err
	}

	process, err := newNormaProcess(agentInstance, closer, flush)
	if err != nil {
		var cleanupErr error
		if closer != nil {
			cleanupErr = closer.Close()
		}

		cleanupErr = errors.Join(cleanupErr, flush())

		return nil, errors.Join(err, cleanupErr)
	}

	return process, nil
}

type normaProcess struct {
	runner   *runner.Runner
	sessions session.Service
	closer   interface{ Close() error }
	flush    func() error
}

func newNormaProcess(agentInstance agent.Agent, closer interface{ Close() error }, flush func() error) (*normaProcess, error) {
	sessions := session.InMemoryService()

	runtimeRunner, err := runner.New(runner.Config{AppName: "callee", Agent: agentInstance, SessionService: sessions})
	if err != nil {
		return nil, fmt.Errorf("create provider runner: %w", err)
	}

	return &normaProcess{runner: runtimeRunner, sessions: sessions, closer: closer, flush: flush}, nil
}

func (p *normaProcess) NewSession(ctx context.Context, role resource.Resource) (AgentSession, error) {
	created, err := p.sessions.Create(ctx, &session.CreateRequest{
		AppName: "callee",
		UserID:  "callee",
		State:   agentSessionState(role),
	})
	if err != nil {
		return nil, fmt.Errorf("create agent %q session: %w", role.ID, err)
	}

	return &normaAgentSession{runner: p.runner, sessionID: created.Session.ID()}, nil
}

func (p *normaProcess) Close() error {
	var errs []error
	if p.closer != nil {
		errs = append(errs, p.closer.Close())
	}

	if p.flush != nil {
		errs = append(errs, p.flush())
	}

	return errors.Join(errs...)
}

type normaAgentSession struct {
	runner    *runner.Runner
	sessionID string
}

func (s *normaAgentSession) Turn(ctx context.Context, prompt string) (string, error) {
	var final string

	for event, err := range s.runner.Run(ctx, "callee", s.sessionID, genai.NewContentFromText(prompt, genai.RoleUser), agent.RunConfig{}) {
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

func (s *normaAgentSession) Prepare(ctx context.Context) error {
	const prepareInput = "callee prepare disposable ACP session"

	// The Norma ACP adapter yields its session binding immediately after
	// session/new and before session/prompt. Returning from the iterator at that
	// event prepares and verifies remote session configuration without inference.
	for event, err := range s.runner.Run(ctx, "callee", s.sessionID, genai.NewContentFromText(prepareInput, genai.RoleUser), agent.RunConfig{}) {
		if err != nil {
			return err
		}

		if _, ok := event.Actions.StateDelta[acpagent.SessionStateKey]; ok {
			return nil
		}

		if event.IsFinalResponse() {
			return fmt.Errorf("provider produced a final response before ACP session binding")
		}
	}

	return fmt.Errorf("provider produced no ACP session binding")
}

func agentSessionState(role resource.Resource) map[string]any {
	provider := role.Spec.Provider
	if provider == nil {
		return nil
	}

	configValues := make([]acpagent.SessionConfigValue, 0, sessionConfigCapacity)
	if model := strings.TrimSpace(provider.Model); model != "" {
		configValues = append(configValues, acpagent.SelectSessionConfigValue("model", model))
	}

	if mode := strings.TrimSpace(provider.Mode); mode != "" {
		configValues = append(configValues, acpagent.SelectSessionConfigValue("mode", mode))
	}

	if reasoning := strings.TrimSpace(provider.Reasoning); reasoning != "" {
		configValues = append(configValues, acpagent.SelectSessionConfigValue("reasoning_effort", reasoning))
	}

	if len(configValues) == 0 {
		return nil
	}

	return map[string]any{
		acpagent.SessionStateKey: map[string]any{"config_values": configValues},
	}
}
