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
)

// Conversation is a long-lived runtime for one role.
type Conversation interface {
	Start(context.Context, string) (threadID, content string, err error)
	Reply(ctx context.Context, threadID, prompt string) (content string, err error)
	Close() error
}

// Factory constructs a role runtime lazily.
type Factory interface {
	New(role.Role) (Conversation, error)
}

// ThreadBinding binds a public Callee thread ID to an internal ACP session.
type ThreadBinding struct {
	ThreadID        string
	RoleID          string
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
	return &Manager{factory: factory, runtimes: map[string]Conversation{}, runtimeIDs: map[string]string{}, threads: map[string]ThreadBinding{}, unavailable: map[string]bool{}}
}

func (m *Manager) runtime(r role.Role) (Conversation, error) {
	if rt := m.runtimes[r.ID]; rt != nil {
		return rt, nil
	}
	rt, err := m.factory.New(r)
	if err != nil {
		return nil, err
	}
	m.nextRuntime++
	m.runtimes[r.ID] = rt
	m.runtimeIDs[r.ID] = fmt.Sprintf("runtime-%d", m.nextRuntime)
	return rt, nil
}

// Start starts a role conversation and returns an opaque Callee thread ID.
func (m *Manager) Start(ctx context.Context, r role.Role, prompt string) (string, string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	rt, err := m.runtime(r)
	if err != nil {
		return "", "", fmt.Errorf("start role %q runtime: %w", r.ID, err)
	}
	backendThreadID, content, err := rt.Start(ctx, prompt)
	if err != nil {
		if isRuntimeCrash(err) {
			m.invalidate(r.ID)
		}
		return "", "", fmt.Errorf("role %q: %w", r.ID, err)
	}
	threadID, err := newThreadID()
	if err != nil {
		return "", "", fmt.Errorf("create thread ID: %w", err)
	}
	m.threads[threadID] = ThreadBinding{ThreadID: threadID, RoleID: r.ID, RuntimeID: m.runtimeIDs[r.ID], BackendThreadID: backendThreadID}
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
	rt := m.runtimes[binding.RoleID]
	if rt == nil || m.runtimeIDs[binding.RoleID] != binding.RuntimeID {
		m.unavailable[threadID] = true
		delete(m.threads, threadID)
		return "", fmt.Errorf("thread %q is no longer available because its runtime was restarted", threadID)
	}
	content, err := rt.Reply(ctx, binding.BackendThreadID, prompt)
	if err != nil {
		if isRuntimeCrash(err) {
			m.invalidate(binding.RoleID)
		}
		return "", fmt.Errorf("role %q: %w", binding.RoleID, err)
	}
	return content, nil
}

// RunOnce executes a role without registering a reusable public thread.
func (m *Manager) RunOnce(ctx context.Context, r role.Role, prompt string) (string, error) {
	rt, err := m.factory.New(r)
	if err != nil {
		return "", fmt.Errorf("start role %q runtime: %w", r.ID, err)
	}
	defer func() { _ = rt.Close() }()
	_, content, err := rt.Start(ctx, prompt)
	if err != nil {
		return "", fmt.Errorf("role %q: %w", r.ID, err)
	}
	return content, nil
}

// Close closes all started role runtimes and discards all bindings.
func (m *Manager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
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

func (m *Manager) invalidate(roleID string) {
	runtimeID := m.runtimeIDs[roleID]
	if rt := m.runtimes[roleID]; rt != nil {
		_ = rt.Close()
	}
	delete(m.runtimes, roleID)
	delete(m.runtimeIDs, roleID)
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
