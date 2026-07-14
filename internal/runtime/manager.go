package runtime

import (
	"context"
	"fmt"
	"sync"

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

// Manager owns long-lived role runtimes and records their live threads.
type Manager struct {
	factory  Factory
	mu       sync.Mutex
	runtimes map[string]Conversation
	threads  map[string]map[string]bool
}

func NewManager(factory Factory) *Manager {
	return &Manager{factory: factory, runtimes: map[string]Conversation{}, threads: map[string]map[string]bool{}}
}

func (m *Manager) runtime(r role.Role) (Conversation, error) {
	if rt := m.runtimes[r.ID]; rt != nil {
		return rt, nil
	}
	rt, err := m.factory.New(r)
	if err != nil {
		return nil, err
	}
	m.runtimes[r.ID] = rt
	m.threads[r.ID] = map[string]bool{}
	return rt, nil
}

// Start starts a new role conversation.
func (m *Manager) Start(ctx context.Context, r role.Role, prompt string) (string, string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	rt, err := m.runtime(r)
	if err != nil {
		return "", "", fmt.Errorf("start role %q runtime: %w", r.ID, err)
	}
	threadID, content, err := rt.Start(ctx, prompt)
	if err != nil {
		return "", "", fmt.Errorf("role %q: %w", r.ID, err)
	}
	m.threads[r.ID][threadID] = true
	return threadID, content, nil
}

// Reply continues an existing role conversation.
func (m *Manager) Reply(ctx context.Context, r role.Role, threadID, prompt string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.threads[r.ID][threadID] {
		return "", fmt.Errorf("thread %q is not available for role %q", threadID, r.ID)
	}
	rt, err := m.runtime(r)
	if err != nil {
		return "", fmt.Errorf("start role %q runtime: %w", r.ID, err)
	}
	content, err := rt.Reply(ctx, threadID, prompt)
	if err != nil {
		return "", fmt.Errorf("role %q: %w", r.ID, err)
	}
	return content, nil
}

// Close closes all started role runtimes.
func (m *Manager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	var first error
	for _, rt := range m.runtimes {
		if err := rt.Close(); err != nil && first == nil {
			first = err
		}
	}
	return first
}
