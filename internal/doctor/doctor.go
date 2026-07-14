// Package doctor checks whether registered Callee roles can initialize ACP runtimes.
package doctor

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"time"

	"github.com/baldaworks/callee/internal/role"
	"github.com/baldaworks/callee/internal/runtime"
)

// Checker initializes and closes one role runtime without sending a model prompt.
type Checker interface {
	Check(ctx context.Context, r role.Role) error
}

// Run checks every provider with an independent timeout and reports every role to stdout.
func Run(ctx context.Context, roles []role.Role, checker Checker, timeout time.Duration, stdout io.Writer) error {
	if checker == nil {
		return errors.New("callee doctor: checker is required")
	}

	if timeout <= 0 {
		return fmt.Errorf("callee doctor: timeout must be greater than zero")
	}

	if len(roles) == 0 {
		return errors.New("callee doctor: no roles found")
	}

	roles = append([]role.Role(nil), roles...)
	sort.Slice(roles, func(i, j int) bool { return roles[i].ID < roles[j].ID })

	type providerGroup struct {
		key   string
		first role.Role
	}

	groupsByKey := make(map[string]providerGroup)
	roleFailures := make(map[string]error)

	for _, r := range roles {
		provider, err := runtime.ProviderFor(r)
		if err != nil {
			roleFailures[r.ID] = err

			continue
		}

		if _, ok := groupsByKey[provider.Key()]; !ok {
			groupsByKey[provider.Key()] = providerGroup{key: provider.Key(), first: r}
		}
	}

	groups := make([]providerGroup, 0, len(groupsByKey))
	for _, group := range groupsByKey {
		groups = append(groups, group)
	}

	sort.Slice(groups, func(i, j int) bool { return groups[i].first.ID < groups[j].first.ID })

	providerFailures := make(map[string]error, len(groups))
	for _, group := range groups {
		roleCtx, cancel := context.WithTimeout(ctx, timeout)
		err := checker.Check(roleCtx, group.first)

		cancel()

		if err != nil {
			providerFailures[group.key] = err
		}
	}

	failures := make([]error, 0)

	for _, r := range roles {
		if err := roleFailures[r.ID]; err != nil {
			failures = append(failures, fmt.Errorf("role %q: %w", r.ID, err))

			continue
		}

		provider, err := runtime.ProviderFor(r)
		if err != nil {
			failures = append(failures, fmt.Errorf("role %q: %w", r.ID, err))

			continue
		}

		if err := providerFailures[provider.Key()]; err != nil {
			failures = append(failures, fmt.Errorf("role %q: %w", r.ID, err))

			continue
		}

		_, _ = fmt.Fprintf(stdout, "role %q: ok\n", r.ID)
	}

	if len(failures) > 0 {
		return fmt.Errorf("callee doctor found %d failing role(s): %w", len(failures), errors.Join(failures...))
	}

	_, _ = fmt.Fprintln(stdout, "callee doctor: ok")

	return nil
}
