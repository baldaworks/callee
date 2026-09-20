// Package doctor validates agent graphs and provider runtimes.
package doctor

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
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
// prompt and verifies Jev credential presence without inference. Static
// resource and graph validation must already have succeeded.
func RunAgents(ctx context.Context, resources []resource.Resource, factory runtime.ProcessFactory, timeout time.Duration, stdout io.Writer) error {
	if timeout <= 0 {
		return fmt.Errorf("callee doctor: timeout must be greater than zero")
	}

	roles, jevs := executableResources(resources)
	if len(roles) == 0 && len(jevs) == 0 {
		return fmt.Errorf("callee doctor: no Role or Jev resources found")
	}

	if len(roles) > 0 && factory == nil {
		return fmt.Errorf("callee doctor: process factory is required")
	}

	resourceFailures := checkJevCredentials(jevs)
	checkRoleResources(ctx, roles, factory, timeout, resourceFailures)

	var failures []error

	checked := append(append([]resource.Resource(nil), roles...), jevs...)
	sort.Slice(checked, func(i, j int) bool { return checked[i].ID < checked[j].ID })

	for _, item := range checked {
		if err := resourceFailures[item.ID]; err != nil {
			failures = append(failures, fmt.Errorf("agent %q: %w", item.ID, err))
		}
	}

	if len(failures) > 0 {
		return fmt.Errorf("callee doctor found %d failing executable resource(s): %w", len(failures), errors.Join(failures...))
	}

	var report bytes.Buffer

	for _, item := range checked {
		_, _ = fmt.Fprintf(&report, "agent %q: ok\n", item.ID)
	}

	_, _ = fmt.Fprintln(&report, "callee doctor: ok")

	if _, err := io.Copy(stdout, &report); err != nil {
		return fmt.Errorf("write doctor report: %w", err)
	}

	return nil
}

func executableResources(resources []resource.Resource) ([]resource.Resource, []resource.Resource) {
	roles := make([]resource.Resource, 0)
	jevs := make([]resource.Resource, 0)

	for _, item := range resources {
		switch item.Kind {
		case resource.RoleKind:
			roles = append(roles, item)
		case resource.JevKind:
			jevs = append(jevs, item)
		}
	}

	sort.Slice(roles, func(i, j int) bool { return roles[i].ID < roles[j].ID })
	sort.Slice(jevs, func(i, j int) bool { return jevs[i].ID < jevs[j].ID })

	return roles, jevs
}

func checkJevCredentials(jevs []resource.Resource) map[string]error {
	failures := make(map[string]error)

	for _, jev := range jevs {
		credential := jevCredential(jev)
		if strings.TrimSpace(os.Getenv(credential)) == "" {
			failures[jev.ID] = fmt.Errorf("environment variable %s is required for Jev API %q", credential, jev.Spec.API.Type)
		}
	}

	return failures
}

func checkRoleResources(
	ctx context.Context,
	roles []resource.Resource,
	factory runtime.ProcessFactory,
	timeout time.Duration,
	resourceFailures map[string]error,
) {
	groupsByKey := make(map[string]*providerGroup)

	for _, role := range roles {
		provider, err := runtime.ProviderForAgent(role)
		if err != nil {
			resourceFailures[role.ID] = err

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
			resourceFailures[roleID] = errors.Join(resourceFailures[roleID], err)
		}
	}
}

func jevCredential(item resource.Resource) string {
	if item.Spec.API != nil && item.Spec.API.Type == "openrouter" {
		return "OPENROUTER_API_KEY"
	}

	return "TYPESAFE_API_KEY"
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

		session, err := process.NewSession(ctx, representative, representative.ID)
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
