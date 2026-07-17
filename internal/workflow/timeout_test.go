package workflow

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestActiveTimeoutPausesOperatorWait(t *testing.T) {
	t.Parallel()

	pauses := NewPauseController()

	ctx, cancel := withActiveTimeout(context.Background(), 80*time.Millisecond, pauses)
	defer cancel()

	time.Sleep(20 * time.Millisecond)

	if err := pauses.Pause(ctx); err != nil {
		t.Fatalf("Pause() error: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	if err := ctx.Err(); err != nil {
		t.Fatalf("active timeout expired while paused: %v", err)
	}

	if err := pauses.Resume(ctx); err != nil {
		t.Fatalf("Resume() error: %v", err)
	}

	select {
	case <-ctx.Done():
		if !errors.Is(ctx.Err(), context.DeadlineExceeded) {
			t.Fatalf("context error = %v, want deadline exceeded", ctx.Err())
		}
	case <-time.After(150 * time.Millisecond):
		t.Fatal("active timeout did not expire after resume")
	}
}
