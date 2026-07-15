package cli

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/baldaworks/callee/internal/role"
	"github.com/baldaworks/callee/internal/runtime"
	"go.uber.org/fx"
)

const shutdownTimeout = 10 * time.Second

type roleRuntimeParams struct {
	fx.In

	Lifecycle fx.Lifecycle
	Context   context.Context
	Factory   runtime.Factory
	Role      role.Role
}

type managedRoleRuntime struct {
	context      context.Context
	cancel       context.CancelFunc
	factory      runtime.Factory
	role         role.Role
	mu           sync.Mutex
	conversation runtime.Conversation
	startDone    chan struct{}
	startCalled  bool
	stopping     bool
	lateCloseErr error
}

func newManagedRoleRuntime(params roleRuntimeParams) *managedRoleRuntime {
	ctx, cancel := context.WithCancel(params.Context)
	managed := &managedRoleRuntime{
		context:   ctx,
		cancel:    cancel,
		factory:   params.Factory,
		role:      params.Role,
		startDone: make(chan struct{}),
	}
	params.Lifecycle.Append(fx.Hook{
		OnStart: managed.start,
		OnStop:  managed.stop,
	})

	return managed
}

func (m *managedRoleRuntime) start(ctx context.Context) error {
	m.mu.Lock()
	m.startCalled = true
	m.mu.Unlock()

	defer close(m.startDone)

	conversation, err := openRole(m.context, m.factory, m.role)
	if err != nil {
		return err
	}

	m.mu.Lock()
	if m.stopping || ctx.Err() != nil {
		m.mu.Unlock()

		closeErr := conversation.Close()
		if closeErr != nil {
			closeErr = fmt.Errorf("close role %q runtime after startup: %w", m.role.ID, closeErr)
		}

		m.mu.Lock()
		m.lateCloseErr = closeErr
		m.mu.Unlock()

		if ctx.Err() != nil {
			return errors.Join(ctx.Err(), closeErr)
		}

		return errors.Join(fmt.Errorf("role %q runtime stopped during startup", m.role.ID), closeErr)
	}

	m.conversation = conversation
	m.mu.Unlock()

	return nil
}

func (m *managedRoleRuntime) stop(ctx context.Context) error {
	m.mu.Lock()
	m.stopping = true
	conversation := m.conversation
	m.conversation = nil
	startCalled := m.startCalled
	m.mu.Unlock()

	if conversation == nil {
		m.cancel()

		if !startCalled {
			return nil
		}

		select {
		case <-m.startDone:
			m.mu.Lock()
			err := m.lateCloseErr
			m.mu.Unlock()

			return err
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	err := conversation.Close()

	m.cancel()

	if err != nil {
		return fmt.Errorf("close role %q runtime: %w", m.role.ID, err)
	}

	return nil
}

func (m *managedRoleRuntime) current() (runtime.Conversation, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.conversation == nil {
		return nil, fmt.Errorf("role %q runtime is not started", m.role.ID)
	}

	return m.conversation, nil
}

type roleLifecycle struct {
	app     *fx.App
	runtime *managedRoleRuntime
}

func newRoleLifecycle(ctx context.Context, factory runtime.Factory, configuredRole role.Role) (*roleLifecycle, error) {
	var managed *managedRoleRuntime

	app := fx.New(
		fx.Provide(func() context.Context { return ctx }),
		fx.Provide(func() runtime.Factory { return factory }),
		fx.Supply(configuredRole),
		fx.Provide(newManagedRoleRuntime),
		fx.Populate(&managed),
		fx.NopLogger,
	)
	if err := app.Err(); err != nil {
		return nil, fmt.Errorf("build role %q lifecycle: %w", configuredRole.ID, err)
	}

	return &roleLifecycle{app: app, runtime: managed}, nil
}

func (l *roleLifecycle) Start(ctx context.Context) error {
	if err := l.app.Start(ctx); err != nil {
		return err
	}

	return nil
}

func (l *roleLifecycle) Stop(ctx context.Context) error {
	appErr := l.app.Stop(ctx)
	runtimeErr := l.runtime.stop(ctx)

	return errors.Join(appErr, runtimeErr)
}

func (l *roleLifecycle) Conversation() (runtime.Conversation, error) {
	return l.runtime.current()
}

type roleLifecycleStopError struct{ err error }

func (e roleLifecycleStopError) Error() string {
	return "stop role lifecycle: " + e.err.Error()
}

func (e roleLifecycleStopError) Unwrap() error {
	return e.err
}

func stopRoleLifecycle(lifecycle *roleLifecycle, resultErr error) error {
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	if err := lifecycle.Stop(shutdownCtx); err != nil {
		return errors.Join(resultErr, roleLifecycleStopError{err: err})
	}

	return resultErr
}
