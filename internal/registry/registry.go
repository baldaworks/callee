// Package registry loads roles from user and project role directories.
package registry

import (
	"fmt"
	"sort"
	"strings"

	"github.com/baldaworks/callee/internal/role"
)

// Registry is an immutable role registry.
type Registry struct{ roles map[string]role.Role }

// New constructs a registry from roles.
func New(roles []role.Role) (*Registry, error) {
	r := &Registry{roles: make(map[string]role.Role, len(roles))}
	for _, item := range roles {
		if _, exists := r.roles[item.ID]; exists {
			return nil, fmt.Errorf("duplicate role %q", item.ID)
		}
		r.roles[item.ID] = item
	}
	return r, nil
}

// Get returns a role by ID.
func (r *Registry) Get(id string) (role.Role, error) {
	item, ok := r.roles[id]
	if !ok {
		available := strings.Join(r.IDs(), ", ")
		return role.Role{}, fmt.Errorf("role %q was not found; available roles: %s", id, available)
	}
	return item, nil
}

// IDs returns sorted role IDs.
func (r *Registry) IDs() []string {
	ids := make([]string, 0, len(r.roles))
	for id := range r.roles {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// Roles returns roles sorted by ID.
func (r *Registry) Roles() []role.Role {
	ids := r.IDs()
	items := make([]role.Role, 0, len(ids))
	for _, id := range ids {
		items = append(items, r.roles[id])
	}
	return items
}
