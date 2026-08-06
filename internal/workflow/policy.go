package workflow

import (
	"fmt"

	"github.com/baldaworks/callee/internal/agent"
	"github.com/baldaworks/callee/internal/registry"
)

// PolicyOverrides contains optional whole-run Role policy overrides.
type PolicyOverrides struct {
	Interactive *bool
	Permissions *agent.PermissionMode
}

// RolePolicy is the effective protocol and ACP permission policy for one Role.
type RolePolicy struct {
	Interactive bool
	Permissions agent.PermissionMode
}

// ResolveRolePolicy resolves independent Role protocol and permission values.
func ResolveRolePolicy(role agent.Resource, overrides PolicyOverrides) (RolePolicy, error) {
	if err := ValidatePolicyOverrides(overrides); err != nil {
		return RolePolicy{}, err
	}

	policy := RolePolicy{
		Interactive: role.Interactive(),
		Permissions: role.EffectivePermissionMode(),
	}
	if overrides.Interactive != nil {
		policy.Interactive = *overrides.Interactive
	}

	if overrides.Permissions != nil {
		policy.Permissions = *overrides.Permissions
	}

	return policy, nil
}

// ValidatePolicyOverrides rejects unsupported programmatic permission modes.
func ValidatePolicyOverrides(overrides PolicyOverrides) error {
	if overrides.Permissions == nil {
		return nil
	}

	switch *overrides.Permissions {
	case agent.PermissionModeAsk, agent.PermissionModeAllow, agent.PermissionModeDeny:
		return nil
	default:
		return fmt.Errorf("permission override %q must be ask, allow, or deny", *overrides.Permissions)
	}
}

// TreeRequiresInteraction reports whether effective tree policy needs an interactor.
func TreeRequiresInteraction(root *registry.ResolvedNode, overrides PolicyOverrides) (bool, error) {
	if err := ValidatePolicyOverrides(overrides); err != nil {
		return false, err
	}

	if root == nil {
		return false, fmt.Errorf("workflow root is required")
	}

	return treeRequiresInteraction(root, overrides), nil
}

func treeRequiresInteraction(node *registry.ResolvedNode, overrides PolicyOverrides) bool {
	switch node.Kind {
	case agent.HumanKind:
		return true
	case agent.RoleKind:
		policy, err := ResolveRolePolicy(node.Resource, overrides)
		if err != nil {
			return false
		}

		if policy.Interactive || policy.Permissions == agent.PermissionModeAsk {
			return true
		}
	}

	for _, child := range node.Children {
		if treeRequiresInteraction(child, overrides) {
			return true
		}
	}

	return false
}
