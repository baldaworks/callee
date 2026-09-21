package workflow

import (
	"fmt"
	"strings"

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

// ResolveNodeRolePolicy resolves policy from a node's registry-projected
// defaults and then applies explicit whole-run overrides.
func ResolveNodeRolePolicy(node *registry.ResolvedNode, overrides PolicyOverrides) (RolePolicy, error) {
	if node == nil || node.Kind != agent.RoleKind {
		return RolePolicy{}, fmt.Errorf("Role node is required")
	}

	if err := ValidatePolicyOverrides(overrides); err != nil {
		return RolePolicy{}, err
	}

	policy := RolePolicy{
		Interactive: node.Interactive != nil && *node.Interactive,
		Permissions: agent.PermissionModeAsk,
	}
	if node.Permissions != nil {
		policy.Permissions = node.Permissions.Mode
	}

	if overrides.Interactive != nil {
		policy.Interactive = *overrides.Interactive
	}

	if overrides.Permissions != nil {
		policy.Permissions = *overrides.Permissions
	}

	return policy, nil
}

// ValidateParallelPreflight rejects interactive policy and missing parameters
// that would otherwise require terminal input from a concurrent subtree.
func ValidateParallelPreflight(root *registry.ResolvedNode, values map[string]string, overrides PolicyOverrides) error {
	if root == nil {
		return fmt.Errorf("workflow root is required")
	}

	if err := ValidatePolicyOverrides(overrides); err != nil {
		return err
	}

	var (
		issues []string
		visit  func(*registry.ResolvedNode)
	)

	visit = func(node *registry.ResolvedNode) {
		if node.Kind == agent.RoleKind && node.WithinParallel {
			boundary := node.ParallelBoundaryID
			if overrides.Interactive != nil && *overrides.Interactive {
				issues = append(issues, fmt.Sprintf("Parallel %q contains Role %q but interactive=true was requested", boundary, node.EffectiveID))
			}

			if overrides.Permissions != nil && *overrides.Permissions == agent.PermissionModeAsk {
				issues = append(issues, fmt.Sprintf("Parallel %q contains Role %q but permissions=ask was requested", boundary, node.EffectiveID))
			}

			for name := range node.Resource.Spec.Params {
				if _, bound := node.Edge.Params[name]; bound {
					continue
				}

				key := node.EffectiveID + "." + name
				if _, supplied := values[key]; !supplied {
					issues = append(issues, fmt.Sprintf("Parallel %q requires parameter %q before fan-out", boundary, key))
				}
			}
		}

		for _, child := range node.Children {
			visit(child)
		}
	}
	visit(root)

	if len(issues) > 0 {
		return fmt.Errorf("Parallel preflight failed: %s", strings.Join(issues, "; "))
	}

	return nil
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
		policy, err := ResolveNodeRolePolicy(node, overrides)
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
