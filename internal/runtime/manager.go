package runtime

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"syscall"

	"github.com/baldaworks/callee/internal/role"
	"github.com/rs/zerolog/log"
)

// Conversation is a long-lived runtime for one ACP provider.
type Conversation interface {
	Start(ctx context.Context, r role.Role, prompt string) (threadID, content string, err error)
	Reply(ctx context.Context, threadID, prompt string) (content string, err error)
	Close() error
}

// Factory constructs a provider runtime.
type Factory interface {
	New(ctx context.Context, provider Provider) (Conversation, error)
}

// ThreadBinding binds a public Callee thread ID to an internal ACP session.
type ThreadBinding struct {
	ThreadID        string
	RoleID          string
	ProviderKey     string
	RuntimeID       string
	BackendThreadID string
}

// Manager owns long-lived role runtimes and opaque public thread bindings.
type Manager struct {
	factory     Factory
	mu          sync.Mutex
	runtimes    map[string]Conversation
	runtimeIDs  map[string]string
	threads     map[string]ThreadBinding
	unavailable map[string]bool
	nextRuntime uint64
}

func NewManager(factory Factory) *Manager {
	return &Manager{
		factory:     factory,
		runtimes:    map[string]Conversation{},
		runtimeIDs:  map[string]string{},
		threads:     map[string]ThreadBinding{},
		unavailable: map[string]bool{},
	}
}

// Start starts a role conversation and returns an opaque Callee thread ID.
func (m *Manager) Start(ctx context.Context, r role.Role, prompt string) (string, string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	log.Debug().Str("role", r.ID).Msg("starting role conversation")

	provider, rt, err := m.runtime(ctx, r)
	if err != nil {
		return "", "", fmt.Errorf("start role %q runtime: %w", r.ID, err)
	}

	backendThreadID, content, err := rt.Start(ctx, r, prompt)
	if err != nil {
		if isRuntimeCrash(err) {
			m.invalidate(provider.Key())
		}

		return "", "", fmt.Errorf("role %q: %w", r.ID, err)
	}

	threadID, err := newThreadID()
	if err != nil {
		return "", "", fmt.Errorf("create thread ID: %w", err)
	}

	m.threads[threadID] = ThreadBinding{ThreadID: threadID, RoleID: r.ID, ProviderKey: provider.Key(), RuntimeID: m.runtimeIDs[provider.Key()], BackendThreadID: backendThreadID}
	log.Debug().Str("role", r.ID).Str("provider", provider.Type()).Str("thread_id", threadID).Str("runtime_id", m.runtimeIDs[provider.Key()]).Msg("started role conversation")

	return threadID, content, nil
}

// Reply continues a public Callee thread without re-resolving its role.
func (m *Manager) Reply(ctx context.Context, threadID, prompt string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	binding, ok := m.threads[threadID]
	if !ok {
		if m.unavailable[threadID] {
			return "", fmt.Errorf("thread %q is no longer available because its runtime was restarted", threadID)
		}

		return "", fmt.Errorf("thread %q was not found", threadID)
	}

	log.Debug().Str("role", binding.RoleID).Str("thread_id", threadID).Str("runtime_id", binding.RuntimeID).Msg("continuing role conversation")

	rt := m.runtimes[binding.ProviderKey]
	if rt == nil || m.runtimeIDs[binding.ProviderKey] != binding.RuntimeID {
		m.unavailable[threadID] = true
		delete(m.threads, threadID)

		return "", fmt.Errorf("thread %q is no longer available because its runtime was restarted", threadID)
	}

	content, err := rt.Reply(ctx, binding.BackendThreadID, prompt)
	if err != nil {
		if isRuntimeCrash(err) {
			m.invalidate(binding.ProviderKey)
		}

		return "", fmt.Errorf("role %q: %w", binding.RoleID, err)
	}

	log.Debug().Str("role", binding.RoleID).Str("thread_id", threadID).Int("content_length", len(content)).Msg("continued role conversation")

	return content, nil
}

// RunOnce executes a role without registering a reusable public thread.
func (m *Manager) RunOnce(ctx context.Context, r role.Role, prompt string) (string, error) {
	log.Debug().Str("role", r.ID).Str("type", r.Metadata.Type).Msg("starting one-shot role runtime")

	provider, err := ProviderFor(r)
	if err != nil {
		return "", fmt.Errorf("start role %q runtime: %w", r.ID, err)
	}

	rt, err := m.factory.New(ctx, provider)
	if err != nil {
		return "", fmt.Errorf("start role %q runtime: %w", r.ID, err)
	}

	defer func() {
		if closeErr := rt.Close(); closeErr != nil {
			log.Debug().Err(closeErr).Str("role", r.ID).Msg("close one-shot role runtime failed")

			return
		}

		log.Debug().Str("role", r.ID).Msg("closed one-shot role runtime")
	}()

	_, content, err := rt.Start(ctx, r, prompt)
	if err != nil {
		return "", fmt.Errorf("role %q: %w", r.ID, err)
	}

	log.Debug().Str("role", r.ID).Int("content_length", len(content)).Msg("one-shot role conversation completed")

	return content, nil
}

// Close closes all started role runtimes and discards all bindings.
func (m *Manager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.closeLocked()
}

// Initialize starts one ACP runtime for every unique provider configuration in
// roles. It is safe to call repeatedly; an already started runtime is reused.
// If startup fails, all runtimes started by this manager are closed.
func (m *Manager) Initialize(ctx context.Context, roles []role.Role) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, r := range roles {
		if _, _, err := m.runtime(ctx, r); err != nil {
			if closeErr := m.closeLocked(); closeErr != nil {
				log.Debug().Err(closeErr).Msg("close ACP runtimes after initialization failure")
			}

			return fmt.Errorf("initialize role %q runtime: %w", r.ID, err)
		}
	}

	return nil
}

func (m *Manager) closeLocked() error {
	var first error
	for _, rt := range m.runtimes {
		if err := rt.Close(); err != nil && first == nil {
			first = err
		}
	}

	m.runtimes = map[string]Conversation{}
	m.runtimeIDs = map[string]string{}
	m.threads = map[string]ThreadBinding{}
	m.unavailable = map[string]bool{}

	return first
}

func (m *Manager) runtime(ctx context.Context, r role.Role) (Provider, Conversation, error) {
	provider, err := ProviderFor(r)
	if err != nil {
		return Provider{}, nil, err
	}

	key := provider.Key()
	if rt := m.runtimes[key]; rt != nil {
		log.Debug().Str("role", r.ID).Str("provider", provider.Type()).
			Str("runtime_id", m.runtimeIDs[key]).Msg("reusing provider runtime")

		return provider, rt, nil
	}

	log.Debug().Str("role", r.ID).Str("provider", provider.Type()).Msg("starting provider runtime")

	rt, err := m.factory.New(ctx, provider)
	if err != nil {
		return Provider{}, nil, err
	}

	m.nextRuntime++
	m.runtimes[key] = rt
	m.runtimeIDs[key] = fmt.Sprintf("runtime-%d", m.nextRuntime)
	log.Debug().Str("role", r.ID).Str("provider", provider.Type()).
		Str("runtime_id", m.runtimeIDs[key]).Msg("provider runtime started")

	return provider, rt, nil
}

func (m *Manager) invalidate(providerKey string) {
	log.Debug().Str("provider_key", providerKey).Str("runtime_id", m.runtimeIDs[providerKey]).Msg("invalidating provider runtime after crash")

	runtimeID := m.runtimeIDs[providerKey]
	if rt := m.runtimes[providerKey]; rt != nil {
		_ = rt.Close()
	}

	delete(m.runtimes, providerKey)
	delete(m.runtimeIDs, providerKey)

	for threadID, binding := range m.threads {
		if binding.RuntimeID == runtimeID {
			delete(m.threads, threadID)
			m.unavailable[threadID] = true
		}
	}
}

func newThreadID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}

	return "cal_" + hex.EncodeToString(value[:]), nil
}

func isRuntimeCrash(err error) bool {
	if errors.Is(err, io.EOF) || errors.Is(err, os.ErrClosed) || errors.Is(err, syscall.EPIPE) {
		return true
	}

	var exitError *exec.ExitError

	return errors.As(err, &exitError)
}
