package runtime

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"

	acpagent "github.com/normahq/go-adk-acpagent/v2"
	"github.com/normahq/runtime/v2/agentconfig"
	"github.com/normahq/runtime/v2/agentfactory"
	"github.com/normahq/runtime/v2/mcpregistry"
	"github.com/rs/zerolog/log"
	"google.golang.org/adk/v2/agent"
)

// NormaFactory builds ACP agents through Norma Runtime.
type NormaFactory struct {
	Stderr            io.Writer
	PermissionHandler acpagent.PermissionHandler
}

var buildNormaAgent = buildNormaAgentDefault

const sessionConfigCapacity = 3

func (f NormaFactory) diagnosticsWriter() (io.Writer, func() error) {
	stderr := f.Stderr
	if stderr == nil {
		stderr = os.Stderr
	}

	return stderr, func() error { return nil }
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

	agentInstance, err := factory.Build(ctx, agentfactory.BuildRequest{
		AgentID:          provider.Key(),
		Name:             provider.Type(),
		Description:      provider.Type(),
		WorkingDirectory: cwd,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("start provider %q runtime: %w", provider.Type(), err)
	}

	closer, _ := agentInstance.(interface{ Close() error })

	log.Debug().Str("provider", provider.Type()).Msg("ACP runtime initialized")

	return agentInstance, closer, nil
}
