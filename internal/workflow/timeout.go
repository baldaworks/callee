package workflow

import (
	"context"
	"sync"
	"time"
)

// PauseController pauses and resumes the active provider-turn budget while
// Callee waits for an operator permission decision.
type PauseController struct {
	events chan bool
}

// NewPauseController creates a controller for one serial workflow run.
func NewPauseController() *PauseController {
	return &PauseController{events: make(chan bool)}
}

// Pause stops the current active-time budget.
func (p *PauseController) Pause(ctx context.Context) error {
	return p.set(ctx, true)
}

// Resume restarts the current active-time budget with its remaining duration.
func (p *PauseController) Resume(ctx context.Context) error {
	return p.set(ctx, false)
}

func (p *PauseController) set(ctx context.Context, paused bool) error {
	select {
	case p.events <- paused:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func withActiveTimeout(parent context.Context, timeout time.Duration, pauses *PauseController) (context.Context, context.CancelFunc) {
	if pauses == nil {
		return context.WithTimeout(parent, timeout)
	}

	ctx := &activeTimeoutContext{
		parent: parent,
		done:   make(chan struct{}),
	}
	cancelled := make(chan struct{})

	go runActiveTimer(ctx, timeout, pauses.events, cancelled)

	var once sync.Once

	cancel := func() {
		once.Do(func() {
			close(cancelled)
		})
	}

	return ctx, cancel
}

type activeTimeoutContext struct {
	parent context.Context
	done   chan struct{}

	mu  sync.Mutex
	err error
}

func (c *activeTimeoutContext) Deadline() (time.Time, bool) { return time.Time{}, false }
func (c *activeTimeoutContext) Done() <-chan struct{}       { return c.done }
func (c *activeTimeoutContext) Value(key any) any           { return c.parent.Value(key) }

func (c *activeTimeoutContext) Err() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.err
}

func (c *activeTimeoutContext) finish(err error) {
	c.mu.Lock()
	if c.err != nil {
		c.mu.Unlock()

		return
	}

	c.err = err
	close(c.done)
	c.mu.Unlock()
}

func runActiveTimer(ctx *activeTimeoutContext, timeout time.Duration, events <-chan bool, cancelled <-chan struct{}) {
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	remaining := timeout
	started := time.Now()
	paused := false

	for {
		select {
		case <-ctx.parent.Done():
			ctx.finish(ctx.parent.Err())

			return
		case <-cancelled:
			ctx.finish(context.Canceled)

			return
		case <-timer.C:
			ctx.finish(context.DeadlineExceeded)

			return
		case pause := <-events:
			switch {
			case pause && !paused:
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}

				remaining -= time.Since(started)
				paused = true
			case !pause && paused:
				if remaining <= 0 {
					ctx.finish(context.DeadlineExceeded)

					return
				}

				started = time.Now()

				timer.Reset(remaining)

				paused = false
			}
		}
	}
}
