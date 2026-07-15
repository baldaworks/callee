package runtime

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/baldaworks/callee/internal/logging"
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
type NormaFactory struct {
	Stderr            io.Writer
	JSONDiagnostics   bool
	PermissionHandler acpagent.PermissionHandler
}

var buildNormaAgent = buildNormaAgentDefault

const sessionConfigCapacity = 3

func (f NormaFactory) New(ctx context.Context, provider Provider) (Conversation, error) {
	stderr, flush := f.diagnosticsWriter()

	ag, closer, err := buildNormaAgent(ctx, provider, stderr, f.PermissionHandler)
	if err != nil {
		if flushErr := flush(); flushErr != nil {
			return nil, errors.Join(err, flushErr)
		}

		return nil, err
	}

	conversation, err := newNormaConversation(ag, closer, flush)
	if err != nil {
		var cleanupErr error
		if closer != nil {
			cleanupErr = closer.Close()
		}

		if flushErr := flush(); flushErr != nil {
			cleanupErr = errors.Join(cleanupErr, flushErr)
		}

		return nil, errors.Join(err, cleanupErr)
	}

	return conversation, nil
}

// Check starts and initializes a role's ACP runtime, then closes it without
// creating a session or sending a model prompt.
func (f NormaFactory) Check(ctx context.Context, r role.Role) error {
	provider, err := ProviderFor(r)
	if err != nil {
		return err
	}

	stderr, flush := f.diagnosticsWriter()

	_, closer, err := buildNormaAgent(ctx, provider, stderr, f.PermissionHandler)
	if err != nil {
		if flushErr := flush(); flushErr != nil {
			return errors.Join(err, flushErr)
		}

		return err
	}

	var closeErr error
	if closer != nil {
		closeErr = closer.Close()
	}

	if flushErr := flush(); flushErr != nil {
		closeErr = errors.Join(closeErr, flushErr)
	}

	if closeErr != nil {
		return fmt.Errorf("close role %q runtime: %w", r.ID, closeErr)
	}

	return nil
}

func (f NormaFactory) diagnosticsWriter() (io.Writer, func() error) {
	stderr := f.Stderr
	if stderr == nil {
		stderr = os.Stderr
	}

	if !f.JSONDiagnostics {
		return stderr, func() error { return nil }
	}

	writer := logging.NewJSONLineWriter(stderr)

	return writer, writer.Flush
}

func buildNormaAgentDefault(ctx context.Context, provider Provider, stderr io.Writer, permissionHandler acpagent.PermissionHandler) (agent.Agent, interface{ Close() error }, error) {
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

	options := []agentfactory.Option{agentfactory.WithStderrWriter(stderr)}
	if permissionHandler != nil {
		options = append(options, agentfactory.WithPermissionHandler(permissionHandler))
	}

	factory := agentfactory.New(map[string]agentconfig.Config{provider.Key(): provider.config}, mcpregistry.New(nil), options...)
	log.Debug().Str("provider", provider.Type()).Msg("initializing ACP runtime")

	ag, err := factory.Build(ctx, agentfactory.BuildRequest{
		AgentID:          provider.Key(),
		Name:             provider.Type(),
		Description:      provider.Type(),
		WorkingDirectory: cwd,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("start provider %q runtime: %w", provider.Type(), err)
	}

	closer, _ := ag.(interface{ Close() error })

	log.Debug().Str("provider", provider.Type()).Msg("ACP runtime initialized")

	return ag, closer, nil
}

type normaConversation struct {
	runner    *runner.Runner
	sessions  session.Service
	closer    interface{ Close() error }
	flush     func() error
	sessionID string
}

func newNormaConversation(ag agent.Agent, closer interface{ Close() error }, flush func() error) (Conversation, error) {
	service := session.InMemoryService()

	r, err := runner.New(runner.Config{AppName: "callee", Agent: ag, SessionService: service})
	if err != nil {
		return nil, err
	}

	return &normaConversation{runner: r, sessions: service, closer: closer, flush: flush}, nil
}

func (n *normaConversation) Run(ctx context.Context, r role.Role, prompt, threadID string) (Result, error) {
	if n.sessionID == "" {
		created, err := n.sessions.Create(ctx, &session.CreateRequest{
			AppName: "callee",
			UserID:  "callee",
			State:   roleSessionState(r, threadID),
		})
		if err != nil {
			return Result{}, err
		}

		n.sessionID = created.Session.ID()
	}

	content, err := n.turn(ctx, n.sessionID, prompt)
	if err != nil {
		return Result{}, err
	}

	remoteThreadID, err := n.acpThreadID(ctx, n.sessionID)
	if err != nil {
		return Result{}, err
	}

	return Result{ThreadID: remoteThreadID, Content: content}, nil
}

func roleSessionState(r role.Role, threadID string) map[string]any {
	configValues := make([]acpagent.SessionConfigValue, 0, sessionConfigCapacity)
	provider := r.Metadata.Provider

	if model := strings.TrimSpace(provider.Model); model != "" {
		configValues = append(configValues, acpagent.SelectSessionConfigValue("model", model))
	}

	if mode := strings.TrimSpace(provider.Mode); mode != "" {
		configValues = append(configValues, acpagent.SelectSessionConfigValue("mode", mode))
	}

	if reasoning := strings.TrimSpace(provider.Reasoning); reasoning != "" {
		configValues = append(configValues, acpagent.SelectSessionConfigValue("reasoning_effort", reasoning))
	}

	if len(configValues) == 0 && strings.TrimSpace(threadID) == "" {
		return nil
	}

	acpState := map[string]any{}
	if len(configValues) > 0 {
		acpState["config_values"] = configValues
	}

	if threadID = strings.TrimSpace(threadID); threadID != "" {
		acpState["session_id"] = threadID
	}

	return map[string]any{acpagent.SessionStateKey: acpState}
}

func (n *normaConversation) Close() error {
	var errs []error
	if n.closer != nil {
		errs = append(errs, n.closer.Close())
	}

	if n.flush != nil {
		errs = append(errs, n.flush())
	}

	return errors.Join(errs...)
}

func (n *normaConversation) acpThreadID(ctx context.Context, id string) (string, error) {
	stored, err := n.sessions.Get(ctx, &session.GetRequest{AppName: "callee", UserID: "callee", SessionID: id})
	if err != nil {
		return "", fmt.Errorf("read role session: %w", err)
	}

	rawState, err := stored.Session.State().Get(acpagent.SessionStateKey)
	if err != nil {
		return "", fmt.Errorf("read ACP session state: %w", err)
	}

	acpState, ok := rawState.(map[string]any)
	if !ok {
		return "", fmt.Errorf("ACP session state must be an object; got %T", rawState)
	}

	threadID, ok := acpState["session_id"].(string)
	if !ok || strings.TrimSpace(threadID) == "" {
		return "", fmt.Errorf("ACP session state has no session_id")
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
