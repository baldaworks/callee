package runtime

import (
	"context"
	"fmt"

	"github.com/baldaworks/callee/internal/role"
)

// Conversation executes one role prompt through an ACP provider.
type Conversation interface {
	Run(ctx context.Context, r role.Role, prompt, threadID string) (Result, error)
	Close() error
}

// Result is the final response from a role invocation. ThreadID is the raw
// ACP session identifier supplied by the provider; Callee does not store it.
type Result struct {
	ThreadID string
	Content  string
}

// Factory constructs a runtime for one role invocation.
type Factory interface {
	New(ctx context.Context, provider Provider) (Conversation, error)
}

// Open starts one role runtime. The caller must close the returned conversation.
func Open(ctx context.Context, factory Factory, r role.Role) (Conversation, error) {
	provider, err := ProviderFor(r)
	if err != nil {
		return nil, fmt.Errorf("start role %q runtime: %w", r.ID, err)
	}

	rt, err := factory.New(ctx, provider)
	if err != nil {
		return nil, fmt.Errorf("start role %q runtime: %w", r.ID, err)
	}

	return rt, nil
}
