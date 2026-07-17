// Package doctor validates agent graphs and provider runtimes.
package doctor

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	resource "github.com/baldaworks/callee/internal/agent"
	"github.com/baldaworks/callee/internal/runtime"
)

const doctorCleanupTimeout = 10 * time.Second

type providerGroup struct {
	key   string
	roles []resource.Resource
}

type sessionGroup struct {
	roles []resource.Resource
}

// RunAgents checks every versioned Role provider group without sending a model
// prompt. Static resource and graph validation must already have succeeded.
func RunAgents(ctx context.Context, resources []resource.Resource, factory runtime.ProcessFactory, timeout time.Duration, stdout io.Writer) error {
	if factory == nil {
		return fmt.Errorf("callee doctor: process factory is required")
	}

	if timeout <= 0 {
		return fmt.Errorf("callee doctor: timeout must be greater than zero")
	}

	roles := make([]resource.Resource, 0)

	for _, item := range resources {
		if item.Kind == resource.RoleKind {
			roles = append(roles, item)
		}
	}

	if len(roles) == 0 {
		return fmt.Errorf("callee doctor: no Role resources found")
	}

	sort.Slice(roles, func(i, j int) bool { return roles[i].ID < roles[j].ID })

	groupsByKey := make(map[string]*providerGroup)
	roleFailures := make(map[string]error)

	for _, role := range roles {
		provider, err := runtime.ProviderForAgent(role)
		if err != nil {
			roleFailures[role.ID] = err

			continue
		}

		group := groupsByKey[provider.Key()]
		if group == nil {
			group = &providerGroup{key: provider.Key()}
			groupsByKey[provider.Key()] = group
		}

		group.roles = append(group.roles, role)
	}

	groups := make([]*providerGroup, 0, len(groupsByKey))
	for _, group := range groupsByKey {
		groups = append(groups, group)
	}

	sort.Slice(groups, func(i, j int) bool { return groups[i].roles[0].ID < groups[j].roles[0].ID })

	for _, group := range groups {
		groupCtx, cancel := context.WithTimeout(ctx, timeout)
		failures := checkAgentGroup(groupCtx, factory, group.roles)

		cancel()

		for roleID, err := range failures {
			roleFailures[roleID] = errors.Join(roleFailures[roleID], err)
		}
	}

	var failures []error

	for _, role := range roles {
		if err := roleFailures[role.ID]; err != nil {
			failures = append(failures, fmt.Errorf("agent %q: %w", role.ID, err))
		}
	}

	if len(failures) > 0 {
		return fmt.Errorf("callee doctor found %d failing Role resource(s): %w", len(failures), errors.Join(failures...))
	}

	var report bytes.Buffer

	for _, role := range roles {
		_, _ = fmt.Fprintf(&report, "agent %q: ok\n", role.ID)
	}

	_, _ = fmt.Fprintln(&report, "callee doctor: ok")

	if _, err := io.Copy(stdout, &report); err != nil {
		return fmt.Errorf("write doctor report: %w", err)
	}

	return nil
}

func checkAgentGroup(ctx context.Context, factory runtime.ProcessFactory, roles []resource.Resource) map[string]error {
	failures := make(map[string]error)

	provider, err := runtime.ProviderForAgent(roles[0])
	if err != nil {
		addGroupFailure(failures, roles, err)

		return failures
	}

	process, err := factory.Start(ctx, provider)
	if err != nil {
		addGroupFailure(failures, roles, err)

		return failures
	}

	groupsByKey := make(map[string]*sessionGroup)

	for _, role := range roles {
		key := sessionConfigKey(role)

		group := groupsByKey[key]
		if group == nil {
			group = &sessionGroup{}
			groupsByKey[key] = group
		}

		group.roles = append(group.roles, role)
	}

	groups := make([]*sessionGroup, 0, len(groupsByKey))
	for _, group := range groupsByKey {
		groups = append(groups, group)
	}

	sort.Slice(groups, func(i, j int) bool { return groups[i].roles[0].ID < groups[j].roles[0].ID })

	for _, group := range groups {
		if err := ctx.Err(); err != nil {
			addGroupFailure(failures, group.roles, err)

			continue
		}

		representative := group.roles[0]

		session, err := process.NewSession(ctx, representative)
		if err != nil {
			addGroupFailure(failures, group.roles, fmt.Errorf("create disposable session for %q: %w", representative.ID, err))

			continue
		}

		if err := session.Prepare(ctx); err != nil {
			addGroupFailure(failures, group.roles, fmt.Errorf("check disposable session for %q: %w", representative.ID, err))
		}
	}

	cleanupCtx, cancel := context.WithTimeout(context.Background(), doctorCleanupTimeout)
	closeErr := closeAgentProcess(cleanupCtx, process)

	cancel()

	if closeErr != nil {
		addGroupFailure(failures, roles, fmt.Errorf("close disposable provider: %w", closeErr))
	}

	return failures
}

func addGroupFailure(failures map[string]error, roles []resource.Resource, err error) {
	for _, role := range roles {
		failures[role.ID] = errors.Join(failures[role.ID], err)
	}
}

func closeAgentProcess(ctx context.Context, process runtime.ProviderProcess) error {
	closed := make(chan error, 1)

	go func() {
		closed <- process.Close()
	}()

	select {
	case err := <-closed:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func sessionConfigKey(role resource.Resource) string {
	provider := role.Spec.Provider
	if provider == nil {
		return role.ID
	}

	return strings.Join([]string{provider.Model, provider.Mode, provider.Reasoning}, "\x00")
}
