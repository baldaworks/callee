// Package registry discovers and resolves versioned Callee agent resources.
package registry

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/baldaworks/callee/internal/agent"
)

// AgentLoadOptions controls versioned agent resource discovery.
type AgentLoadOptions struct {
	UserDir      string
	ProjectDir   string
	HomeDir      string
	ExclusiveDir string
}

// AgentRegistry is an immutable registry of versioned agent resources.
type AgentRegistry struct {
	resources map[string]agent.Resource
}

// RequiredParam describes one statically unbound reachable Role parameter.
type RequiredParam struct {
	EffectiveID  string `json:"effectiveId"`
	Name         string `json:"name"`
	Description  string `json:"description"`
	Key          string `json:"key"`
	SourceRoleID string `json:"sourceRoleId"`
}

// ResolvedNode is one occurrence in a selected root execution tree.
type ResolvedNode struct {
	EffectiveID         string             `json:"effectiveId"`
	ResourceID          string             `json:"resourceId"`
	Kind                agent.Kind         `json:"kind"`
	CanEscalate         bool               `json:"canEscalate"`
	Permissions         *agent.Permissions `json:"permissions,omitempty"`
	AuthoredPermissions *agent.Permissions `json:"authoredPermissions,omitempty"`
	Interactive         *bool              `json:"interactive,omitempty"`
	REPL                *bool              `json:"repl,omitempty"`
	MaxIterations       *int               `json:"maxIterations,omitempty"`
	OnExhausted         string             `json:"onExhausted,omitempty"`
	Children            []*ResolvedNode    `json:"children"`

	Resource agent.Resource `json:"-"`
	Edge     agent.Child    `json:"-"`
	Path     []string       `json:"-"`
}

// LoadAgents discovers versioned resources under the user Callee root and the
// project .callee tree. Duplicate IDs are fatal; project resources never
// shadow user resources.
func LoadAgents(opts AgentLoadOptions) (*AgentRegistry, error) {
	if opts.ExclusiveDir != "" {
		return loadAgentRoots([]agentRoot{{label: "exclusive", path: opts.ExclusiveDir}})
	}

	if opts.HomeDir == "" {
		opts.HomeDir, _ = os.UserHomeDir()
	}

	if opts.UserDir == "" {
		base := os.Getenv("XDG_CONFIG_HOME")
		if base == "" {
			base = filepath.Join(opts.HomeDir, ".config")
		}

		opts.UserDir = filepath.Join(base, "callee")
	}

	if opts.ProjectDir == "" {
		opts.ProjectDir = ".callee"
	}

	return loadAgentRoots([]agentRoot{
		{label: "user", path: opts.UserDir},
		{label: "project", path: opts.ProjectDir},
	})
}

// NewAgentRegistry constructs and statically validates a registry.
func NewAgentRegistry(resources []agent.Resource) (*AgentRegistry, error) {
	ordered := append([]agent.Resource(nil), resources...)
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].ID != ordered[j].ID {
			return ordered[i].ID < ordered[j].ID
		}

		return ordered[i].Source < ordered[j].Source
	})

	var diagnostics []error

	candidates := make(map[string]agent.Resource, len(ordered))
	invalid := make(map[string]bool)
	duplicates := make(map[string]bool)

	for _, resource := range ordered {
		if err := resource.Validate(); err != nil {
			diagnostics = append(diagnostics, err)
			invalid[resource.ID] = true
		}

		if previous, exists := candidates[resource.ID]; exists {
			diagnostics = append(diagnostics, fmt.Errorf("duplicate agent ID %q from %q and %q", resource.ID, previous.Source, resource.Source))
			duplicates[resource.ID] = true

			continue
		}

		candidates[resource.ID] = resource
	}

	registry := &AgentRegistry{resources: make(map[string]agent.Resource, len(candidates))}
	for id, resource := range candidates {
		if invalid[id] || duplicates[id] {
			continue
		}

		registry.resources[id] = resource
	}

	for _, id := range registry.staticRoots() {
		if _, err := registry.Resolve(id); err != nil {
			diagnostics = append(diagnostics, err)
		}
	}

	if len(diagnostics) > 0 {
		return nil, fmt.Errorf("agent registry has %d static error(s): %w", len(diagnostics), errors.Join(diagnostics...))
	}

	return registry, nil
}

// GetAgent returns a resource by ID.
func (r *AgentRegistry) GetAgent(id string) (agent.Resource, error) {
	resource, ok := r.resources[id]
	if !ok {
		return agent.Resource{}, fmt.Errorf("agent %q was not found; available agents: %s", id, strings.Join(r.IDs(), ", "))
	}

	return resource, nil
}

// IDs returns resource IDs in lexicographic order.
func (r *AgentRegistry) IDs() []string {
	ids := make([]string, 0, len(r.resources))
	for id := range r.resources {
		ids = append(ids, id)
	}

	sort.Strings(ids)

	return ids
}

// Agents returns resources in resource-ID order.
func (r *AgentRegistry) Agents() []agent.Resource {
	resources := make([]agent.Resource, 0, len(r.resources))
	for _, id := range r.IDs() {
		resources = append(resources, r.resources[id])
	}

	return resources
}

// Resolve builds and validates one concrete root execution tree.
func (r *AgentRegistry) Resolve(id string) (*ResolvedNode, error) {
	resource, err := r.GetAgent(id)
	if err != nil {
		return nil, err
	}

	effectiveIDs := make(map[string]string)
	stack := make(map[string]string)

	return r.resolve(resource, agent.Child{}, effectiveIDs, stack, nil, false, false)
}

func (r *AgentRegistry) staticRoots() []string {
	referenced := make(map[string]bool)

	for _, resource := range r.resources {
		for _, child := range resource.Spec.Children {
			referenced[child.Ref] = true
		}
	}

	var roots []string

	for _, id := range r.IDs() {
		if !referenced[id] {
			roots = append(roots, id)
		}
	}

	visited := make(map[string]bool)

	var visit func(string)

	visit = func(id string) {
		if visited[id] {
			return
		}

		visited[id] = true

		resource, ok := r.resources[id]
		if !ok {
			return
		}

		for _, child := range resource.Spec.Children {
			visit(child.Ref)
		}
	}

	for _, id := range roots {
		visit(id)
	}

	for _, id := range r.IDs() {
		if !visited[id] {
			roots = append(roots, id)
			visit(id)
		}
	}

	return roots
}

func (r *AgentRegistry) resolve(
	resource agent.Resource,
	edge agent.Child,
	effectiveIDs, stack map[string]string,
	parentPath []string,
	withinLoop, canEscalate bool,
) (*ResolvedNode, error) {
	if stack[resource.ID] != "" {
		return nil, fmt.Errorf("agent graph cycle: %s -> %s", stack[resource.ID], resource.ID)
	}

	effectiveID := resource.ID
	if edge.Alias != "" {
		effectiveID = edge.Alias
	}

	path := append(append([]string(nil), parentPath...), effectiveID)

	if previous := effectiveIDs[effectiveID]; previous != "" {
		return nil, fmt.Errorf("resolved agent tree has duplicate effective ID %q at %s and %s", effectiveID, previous, resource.ID)
	}

	effectiveIDs[effectiveID] = resource.ID
	stack[resource.ID] = effectiveID

	node := &ResolvedNode{
		EffectiveID: effectiveID,
		ResourceID:  resource.ID,
		Kind:        resource.Kind,
		CanEscalate: canEscalate,
		Children:    make([]*ResolvedNode, 0, len(resource.Spec.Children)),
		Resource:    resource,
		Edge:        edge,
		Path:        path,
	}

	switch resource.Kind {
	case agent.RoleKind:
		interactive := resource.Interactive()
		node.Interactive = &interactive
		node.REPL = &interactive
		node.Permissions = &agent.Permissions{Mode: resource.EffectivePermissionMode()}
		node.AuthoredPermissions = resource.Spec.Permissions
	case agent.LoopKind:
		node.MaxIterations = resource.Spec.MaxIterations
		node.OnExhausted = resource.ExhaustionPolicy()
	}

	for index, child := range resource.Spec.Children {
		childWithinLoop := withinLoop
		childCanEscalate := canEscalate && child.CanEscalate

		if resource.Kind == agent.LoopKind {
			childWithinLoop = true
			childCanEscalate = child.CanEscalate
		}

		if child.CanEscalate && !childWithinLoop {
			return nil, fmt.Errorf("agent %q child %d (%q): canEscalate is only valid beneath a Loop", resource.ID, index, strings.Join(append(path, child.Ref), " -> "))
		}

		childResource, err := r.GetAgent(child.Ref)
		if err != nil {
			return nil, fmt.Errorf("agent %q child %d: %w", resource.ID, index, err)
		}

		if err := validateChildParams(resource.ID, index, child, childResource); err != nil {
			return nil, err
		}

		resolved, err := r.resolve(childResource, child, effectiveIDs, stack, path, childWithinLoop, childCanEscalate)
		if err != nil {
			return nil, fmt.Errorf("agent %q child %d: %w", resource.ID, index, err)
		}

		node.Children = append(node.Children, resolved)
	}

	delete(stack, resource.ID)

	return node, nil
}

// RequiredParams returns every statically unbound Role parameter in preorder.
func RequiredParams(root *ResolvedNode) []RequiredParam {
	var required []RequiredParam

	var visit func(*ResolvedNode)

	visit = func(node *ResolvedNode) {
		if node.Kind == agent.RoleKind {
			names := make([]string, 0, len(node.Resource.Spec.Params))
			for name := range node.Resource.Spec.Params {
				if _, bound := node.Edge.Params[name]; !bound {
					names = append(names, name)
				}
			}

			sort.Strings(names)

			for _, name := range names {
				required = append(required, RequiredParam{
					EffectiveID:  node.EffectiveID,
					Name:         name,
					Description:  strings.TrimSpace(node.Resource.Spec.Params[name]),
					Key:          node.EffectiveID + "." + name,
					SourceRoleID: node.ResourceID,
				})
			}
		}

		for _, child := range node.Children {
			visit(child)
		}
	}

	visit(root)

	return required
}

func validateChildParams(parentID string, index int, child agent.Child, resource agent.Resource) error {
	if len(child.Params) == 0 {
		return nil
	}

	if resource.Kind != agent.RoleKind {
		return fmt.Errorf("agent %q child %d: params are valid only when %q resolves to Role", parentID, index, child.Ref)
	}

	for name := range child.Params {
		if _, ok := resource.Spec.Params[name]; !ok {
			return fmt.Errorf("agent %q child %d: Role %q does not declare parameter %q", parentID, index, child.Ref, name)
		}
	}

	return nil
}

type agentRoot struct {
	label string
	path  string
}

type discoveredAgent struct {
	resource agent.Resource
	info     os.FileInfo
	label    string
	path     string
}

func loadAgentRoots(roots []agentRoot) (*AgentRegistry, error) {
	var (
		byID        = make(map[string]discoveredAgent)
		physical    []discoveredAgent
		diagnostics []error
	)

	for _, root := range roots {
		discovered, discoveredDiagnostics := discoverAgentRoot(root)
		diagnostics = append(diagnostics, discoveredDiagnostics...)

		for _, item := range discovered {
			if previous, ok := byID[item.resource.ID]; ok {
				diagnostics = append(diagnostics, fmt.Errorf("duplicate agent ID %q: %s %q and %s %q", item.resource.ID, previous.label, previous.path, item.label, item.path))

				continue
			}

			duplicatePhysical := false

			for _, previous := range physical {
				if os.SameFile(previous.info, item.info) {
					diagnostics = append(diagnostics, fmt.Errorf("agent file %q is also discovered as %q", item.path, previous.path))
					duplicatePhysical = true

					break
				}
			}

			if duplicatePhysical {
				continue
			}

			byID[item.resource.ID] = item
			physical = append(physical, item)
		}
	}

	resources := make([]agent.Resource, 0, len(byID))
	for _, item := range byID {
		resources = append(resources, item.resource)
	}

	registry, err := NewAgentRegistry(resources)
	if err != nil {
		diagnostics = append(diagnostics, err)
	}

	if len(diagnostics) > 0 {
		return nil, fmt.Errorf("agent discovery has %d static error(s): %w", len(diagnostics), errors.Join(diagnostics...))
	}

	return registry, nil
}

func discoverAgentRoot(root agentRoot) ([]discoveredAgent, []error) {
	resolved, err := filepath.EvalSymlinks(root.path)
	if os.IsNotExist(err) {
		return nil, nil
	}

	if err != nil {
		return nil, []error{fmt.Errorf("resolve %s agent root %q: %w", root.label, root.path, err)}
	}

	var (
		discovered  []discoveredAgent
		diagnostics []error
	)

	err = filepath.WalkDir(resolved, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			diagnostics = append(diagnostics, fmt.Errorf("visit agent path %q: %w", path, walkErr))

			if entry == nil || entry.IsDir() {
				return filepath.SkipDir
			}

			return nil
		}

		if entry.IsDir() {
			return nil
		}

		if entry.Type()&os.ModeSymlink != 0 || !entry.Type().IsRegular() || !agent.SupportsFile(entry.Name()) {
			return nil
		}

		relative, err := filepath.Rel(resolved, path)
		if err != nil {
			diagnostics = append(diagnostics, fmt.Errorf("resolve agent path %q relative to %q: %w", path, resolved, err))

			return nil
		}

		id, err := ResourceID(relative)
		if err != nil {
			diagnostics = append(diagnostics, fmt.Errorf("resource path %q: %w", path, err))

			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			diagnostics = append(diagnostics, fmt.Errorf("read agent %q: %w", path, err))

			return nil
		}

		resource, err := agent.Decode(id, path, data)
		if err != nil {
			diagnostics = append(diagnostics, fmt.Errorf("parse %q: %w", path, err))

			return nil
		}

		info, err := entry.Info()
		if err != nil {
			diagnostics = append(diagnostics, fmt.Errorf("stat agent %q: %w", path, err))

			return nil
		}

		discovered = append(discovered, discoveredAgent{resource: resource, info: info, label: root.label, path: path})

		return nil
	})
	if err != nil {
		diagnostics = append(diagnostics, fmt.Errorf("discover %s agents under %q: %w", root.label, root.path, err))
	}

	sort.Slice(discovered, func(i, j int) bool {
		if discovered[i].resource.ID != discovered[j].resource.ID {
			return discovered[i].resource.ID < discovered[j].resource.ID
		}

		return discovered[i].path < discovered[j].path
	})

	return discovered, diagnostics
}

// ResourceID derives a Callee resource ID from a discovery-root-relative path.
func ResourceID(relative string) (string, error) {
	clean := filepath.Clean(relative)
	if clean == "." || filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes its discovery root")
	}

	if !agent.SupportsFile(clean) {
		return "", fmt.Errorf("unsupported agent file extension %q", filepath.Ext(clean))
	}

	id := strings.TrimSuffix(filepath.ToSlash(clean), filepath.Ext(clean))
	for _, segment := range strings.Split(id, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return "", fmt.Errorf("invalid empty or dot path segment")
		}
	}

	return id, nil
}
