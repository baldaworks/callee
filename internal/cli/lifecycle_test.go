package cli

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/baldaworks/callee/internal/role"
	"github.com/baldaworks/callee/internal/runtime"
)

type lifecycleConversation struct {
	closed    chan struct{}
	closeWait <-chan struct{}
	closeErr  error
}

func (c *lifecycleConversation) Run(context.Context, role.Role, string, string) (runtime.Result, error) {
	return runtime.Result{}, nil
}

func (c *lifecycleConversation) Close() error {
	if c.closeWait != nil {
		<-c.closeWait
	}

	select {
	case <-c.closed:
	default:
		close(c.closed)
	}

	return c.closeErr
}

func testLifecycleRole() role.Role {
	return role.Role{
		ID: "reviewer",
		Metadata: role.Metadata{
			Provider: role.Provider{Type: "codex"},
		},
	}
}

func TestRoleLifecycleStartsAndStopsConversation(t *testing.T) {
	original := openRole

	t.Cleanup(func() { openRole = original })

	var lifetimeCtx context.Context

	conversation := &lifecycleConversation{closed: make(chan struct{})}

	openRole = func(ctx context.Context, _ runtime.Factory, configuredRole role.Role) (runtime.Conversation, error) {
		lifetimeCtx = ctx

		if configuredRole.ID != "reviewer" {
			t.Fatalf("role ID = %q, want reviewer", configuredRole.ID)
		}

		return conversation, nil
	}

	lifecycle, err := newRoleLifecycle(context.Background(), nil, testLifecycleRole())
	if err != nil {
		t.Fatal(err)
	}

	if err := lifecycle.Start(context.Background()); err != nil {
		t.Fatal(err)
	}

	got, err := lifecycle.Conversation()
	if err != nil {
		t.Fatal(err)
	}

	if got != conversation {
		t.Fatalf("conversation = %T, want lifecycle conversation", got)
	}

	if err := lifecycle.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}

	select {
	case <-conversation.closed:
	default:
		t.Fatal("conversation was not closed")
	}

	select {
	case <-lifetimeCtx.Done():
	default:
		t.Fatal("runtime lifetime context was not cancelled")
	}
}

func TestRoleLifecycleReturnsCloseError(t *testing.T) {
	original := openRole

	t.Cleanup(func() { openRole = original })

	wantErr := errors.New("close failed")
	conversation := &lifecycleConversation{closed: make(chan struct{}), closeErr: wantErr}

	openRole = func(context.Context, runtime.Factory, role.Role) (runtime.Conversation, error) {
		return conversation, nil
	}

	lifecycle, err := newRoleLifecycle(context.Background(), nil, testLifecycleRole())
	if err != nil {
		t.Fatal(err)
	}

	if err := lifecycle.Start(context.Background()); err != nil {
		t.Fatal(err)
	}

	err = lifecycle.Stop(context.Background())
	if !errors.Is(err, wantErr) {
		t.Fatalf("Stop() error = %v, want close error", err)
	}
}

func TestRoleLifecycleStopHonorsContext(t *testing.T) {
	original := openRole

	t.Cleanup(func() { openRole = original })

	release := make(chan struct{})
	conversation := &lifecycleConversation{closed: make(chan struct{}), closeWait: release}

	openRole = func(context.Context, runtime.Factory, role.Role) (runtime.Conversation, error) {
		return conversation, nil
	}

	lifecycle, err := newRoleLifecycle(context.Background(), nil, testLifecycleRole())
	if err != nil {
		t.Fatal(err)
	}

	if err := lifecycle.Start(context.Background()); err != nil {
		t.Fatal(err)
	}

	stopCtx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	err = lifecycle.Stop(stopCtx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Stop() error = %v, want deadline exceeded", err)
	}

	close(release)

	select {
	case <-conversation.closed:
	case <-time.After(time.Second):
		t.Fatal("conversation close did not finish")
	}
}

func TestRoleLifecycleClosesRuntimeThatStartsAfterTimeout(t *testing.T) {
	original := openRole

	t.Cleanup(func() { openRole = original })

	started := make(chan struct{})
	release := make(chan struct{})
	conversation := &lifecycleConversation{closed: make(chan struct{})}

	openRole = func(context.Context, runtime.Factory, role.Role) (runtime.Conversation, error) {
		close(started)
		<-release

		return conversation, nil
	}

	lifecycle, err := newRoleLifecycle(context.Background(), nil, testLifecycleRole())
	if err != nil {
		t.Fatal(err)
	}

	startCtx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	startErr := make(chan error, 1)
	go func() {
		startErr <- lifecycle.Start(startCtx)
	}()

	<-started

	if err := <-startErr; !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Start() error = %v, want deadline exceeded", err)
	}

	stopErr := make(chan error, 1)
	go func() {
		stopErr <- lifecycle.Stop(context.Background())
	}()

	select {
	case err := <-stopErr:
		t.Fatalf("Stop() returned before startup finished: %v", err)
	case <-time.After(20 * time.Millisecond):
	}

	close(release)

	select {
	case <-conversation.closed:
	case <-time.After(time.Second):
		t.Fatal("late runtime was not closed")
	}

	if err := <-stopErr; err != nil {
		t.Fatal(err)
	}
}

func TestRoleLifecycleReturnsLateCloseError(t *testing.T) {
	original := openRole

	t.Cleanup(func() { openRole = original })

	started := make(chan struct{})
	release := make(chan struct{})
	wantErr := errors.New("late close failed")
	conversation := &lifecycleConversation{closed: make(chan struct{}), closeErr: wantErr}

	openRole = func(context.Context, runtime.Factory, role.Role) (runtime.Conversation, error) {
		close(started)
		<-release

		return conversation, nil
	}

	lifecycle, err := newRoleLifecycle(context.Background(), nil, testLifecycleRole())
	if err != nil {
		t.Fatal(err)
	}

	startCtx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	startErr := make(chan error, 1)
	go func() {
		startErr <- lifecycle.Start(startCtx)
	}()

	<-started

	if err := <-startErr; !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Start() error = %v, want deadline exceeded", err)
	}

	close(release)

	if err := lifecycle.Stop(context.Background()); !errors.Is(err, wantErr) {
		t.Fatalf("Stop() error = %v, want late close error", err)
	}
}

func TestExpectedCancellationRejectsLifecycleStopError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	closeErr := errors.New("close failed")

	err := errors.Join(context.Canceled, roleLifecycleStopError{err: closeErr})
	if isExpectedCancellation(ctx, err) {
		t.Fatalf("isExpectedCancellation(%v) = true, want false", err)
	}
}
