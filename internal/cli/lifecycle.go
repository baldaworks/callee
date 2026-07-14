package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/baldaworks/callee/internal/registry"
	"github.com/baldaworks/callee/internal/runtime"
	"github.com/baldaworks/callee/internal/server"
	"go.uber.org/fx"
)

const shutdownTimeout = 10 * time.Second

type mcpServerParams struct {
	fx.In

	Lifecycle fx.Lifecycle
	Registry  *registry.Registry
	Manager   *runtime.Manager
}

func newMCPManager(factory runtime.Factory) *runtime.Manager {
	return runtime.NewManager(factory)
}

func newMCPServer(params mcpServerParams) *server.MCP {
	params.Lifecycle.Append(fx.Hook{OnStop: func(context.Context) error {
		return params.Manager.Close()
	}})

	return server.New(params.Registry, params.Manager)
}

func mcpOptions(reg *registry.Registry, factory runtime.Factory, target **server.MCP) fx.Option {
	return fx.Options(
		fx.Supply(reg),
		fx.Provide(func() runtime.Factory { return factory }),
		fx.Provide(newMCPManager, newMCPServer),
		fx.Populate(target),
		fx.NopLogger,
	)
}

func runMCPServer(ctx context.Context, reg *registry.Registry, version string) (err error) {
	var mcpServer *server.MCP

	app := fx.New(mcpOptions(reg, runtime.NormaFactory{}, &mcpServer))
	if err := app.Err(); err != nil {
		return fmt.Errorf("build MCP lifecycle: %w", err)
	}

	if err := app.Start(ctx); err != nil {
		return fmt.Errorf("start MCP lifecycle: %w", err)
	}

	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()

		if stopErr := app.Stop(shutdownCtx); stopErr != nil && err == nil {
			err = fmt.Errorf("stop MCP lifecycle: %w", stopErr)
		}
	}()

	err = mcpServer.RunStdio(ctx, version)
	if isExpectedCancellation(ctx, err) {
		return nil
	}

	return err
}
