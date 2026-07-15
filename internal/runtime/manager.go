package runtime

import (
	"context"
	"fmt"

	"github.com/baldaworks/callee/internal/role"
	"github.com/rs/zerolog/log"
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

// RunOnce executes a role and closes its runtime before returning.
func RunOnce(ctx context.Context, factory Factory, r role.Role, prompt, threadID string) (Result, error) {
	log.Debug().Str("role", r.ID).Str("type", r.Metadata.Type).Msg("starting one-shot role runtime")

	provider, err := ProviderFor(r)
	if err != nil {
		return Result{}, fmt.Errorf("start role %q runtime: %w", r.ID, err)
	}

	rt, err := factory.New(ctx, provider)
	if err != nil {
		return Result{}, fmt.Errorf("start role %q runtime: %w", r.ID, err)
	}

	defer func() {
		if closeErr := rt.Close(); closeErr != nil {
			log.Debug().Err(closeErr).Str("role", r.ID).Msg("close one-shot role runtime failed")

			return
		}

		log.Debug().Str("role", r.ID).Msg("closed one-shot role runtime")
	}()

	result, err := rt.Run(ctx, r, prompt, threadID)
	if err != nil {
		return Result{}, fmt.Errorf("role %q: %w", r.ID, err)
	}

	log.Debug().Str("role", r.ID).Int("content_length", len(result.Content)).Msg("one-shot role conversation completed")

	return result, nil
}
