package workflow

import (
	"bytes"
	"context"
	"errors"
	"math"
	"strings"
	"testing"

	"github.com/baldaworks/callee/internal/agent"
	"github.com/baldaworks/callee/internal/registry"
	"github.com/baldaworks/callee/internal/runtime"
	"github.com/rs/zerolog"
	"google.golang.org/adk/v2/session"
)

func TestCollectRootRunOutputSelectsTypedTerminalEnvelope(t *testing.T) {
	t.Parallel()

	want := rootRunOutput{execution: nodeExecution{result: nodeResult{artifact: "done"}}}
	events := func(yield func(*session.Event, error) bool) {
		if !yield(&session.Event{
			Output:   nodeResult{artifact: "not terminal"},
			NodeInfo: &session.NodeInfo{Path: "callee_node_999999"},
		}, nil) {
			return
		}

		yield(&session.Event{
			Output:   want,
			NodeInfo: &session.NodeInfo{Path: "an/arbitrary/adk/path"},
		}, nil)
	}

	got, err := collectRootRunOutput(context.Background(), events)
	if err != nil {
		t.Fatalf("collectRootRunOutput() error: %v", err)
	}

	if got.execution.result.artifact != want.execution.result.artifact {
		t.Fatalf("collectRootRunOutput() artifact = %q, want %q", got.execution.result.artifact, want.execution.result.artifact)
	}
}

func TestCollectRootRunOutputRequiresExactlyOneTerminalEnvelope(t *testing.T) {
	t.Parallel()

	for _, count := range []int{0, 2} {
		t.Run(strings.Repeat("terminal_", count), func(t *testing.T) {
			t.Parallel()

			events := func(yield func(*session.Event, error) bool) {
				for range count {
					if !yield(&session.Event{Output: rootRunOutput{}}, nil) {
						return
					}
				}
			}

			_, err := collectRootRunOutput(context.Background(), events)
			if err == nil || !strings.Contains(err.Error(), "want exactly one") {
				t.Fatalf("collectRootRunOutput() error = %v, want terminal count error", err)
			}
		})
	}
}

func TestADKCompilerNamesAreDeterministicAndPathSafe(t *testing.T) {
	t.Parallel()

	compiler := &adkCompiler{logger: zerolog.Nop()}

	first, err := compiler.nextNodeName(adkNodeRoleResource, "Role", "roles/Worker")
	if err != nil {
		t.Fatalf("nextNodeName() first error: %v", err)
	}

	if want := "callee_0000000000000001_resource_role_roles_worker"; first != want {
		t.Errorf("nextNodeName() first = %q, want %q", first, want)
	}

	second, err := compiler.nextNodeName(adkNodeRoleResource, "Role", "roles Worker")
	if err != nil {
		t.Fatalf("nextNodeName() second error: %v", err)
	}

	if first == second {
		t.Fatalf("normalized collisions reused node name %q", first)
	}

	long, err := compiler.nextNodeName(adkNodeRoleRouterBranch, strings.Repeat("very/long alias ", 32))
	if err != nil {
		t.Fatalf("nextNodeName() long error: %v", err)
	}

	if len(long) > maxADKNodeNameLength {
		t.Errorf("long node name length = %d, want at most %d: %q", len(long), maxADKNodeNameLength, long)
	}

	for _, name := range []string{first, second, long} {
		for _, char := range name {
			if (char < 'a' || char > 'z') && (char < '0' || char > '9') && char != '_' {
				t.Errorf("node name %q contains unsafe character %q", name, char)
			}
		}
	}

	repeated := &adkCompiler{logger: zerolog.Nop()}

	repeatedFirst, err := repeated.nextNodeName(adkNodeRoleResource, "Role", "roles/Worker")
	if err != nil {
		t.Fatalf("repeated nextNodeName() error: %v", err)
	}

	if repeatedFirst != first {
		t.Errorf("repeated compiler name = %q, want deterministic %q", repeatedFirst, first)
	}

	if got, want := sanitizeADKNodeNamePart(" / Réview/步骤 !!! "), "r_view"; got != want {
		t.Errorf("sanitizeADKNodeNamePart() = %q, want %q", got, want)
	}

	if got, want := sanitizeADKNodeNamePart("步骤"), "node"; got != want {
		t.Errorf("sanitizeADKNodeNamePart() Unicode-only = %q, want %q", got, want)
	}
}

func TestADKCompilerRejectsNodeOrdinalExhaustion(t *testing.T) {
	t.Parallel()

	compiler := &adkCompiler{logger: zerolog.Nop(), next: math.MaxUint64 - 1}
	if _, err := compiler.nextNodeName(adkNodeRoleTerminal, "root"); err != nil {
		t.Fatalf("nextNodeName() at final ordinal error: %v", err)
	}

	if _, err := compiler.nextNodeName(adkNodeRoleTerminal, "root"); err == nil || !strings.Contains(err.Error(), "ordinal exhausted") {
		t.Fatalf("nextNodeName() after exhaustion error = %v, want exhaustion", err)
	}
}

func TestADKCompilerLogsStructuredResourceAndHelperIdentities(t *testing.T) {
	t.Parallel()

	const secret = "secret-prompt-and-route"

	owner := &registry.ResolvedNode{
		EffectiveID: "review router",
		ResourceID:  "workflows/review",
		Kind:        agent.RouterKind,
		Resource: agent.Resource{
			Spec: agent.Spec{Body: secret},
		},
	}
	child := &registry.ResolvedNode{
		EffectiveID: "reviewer",
		ResourceID:  "roles/reviewer",
		Kind:        agent.RoleKind,
		Edge:        agent.Child{Route: secret, Input: secret},
	}
	direct := &registry.ResolvedNode{
		EffectiveID: "scripts/check",
		ResourceID:  "scripts/check",
		Kind:        agent.ScriptKind,
	}

	var output bytes.Buffer

	compiler := &adkCompiler{logger: zerolog.New(&output).Level(zerolog.DebugLevel)}
	if _, err := compiler.resourceName(owner); err != nil {
		t.Fatalf("resourceName(owner) error: %v", err)
	}

	if _, err := compiler.resourceName(direct); err != nil {
		t.Fatalf("resourceName(direct) error: %v", err)
	}

	if _, err := compiler.helperName(adkNodeRoleTerminal, nil, nil); err != nil {
		t.Fatalf("helperName(terminal) error: %v", err)
	}

	if _, err := compiler.helperName(adkNodeRoleRouterDispatch, owner, nil); err != nil {
		t.Fatalf("helperName(dispatch) error: %v", err)
	}

	if _, err := compiler.helperName(adkNodeRoleRouterBranch, owner, child); err != nil {
		t.Fatalf("helperName(branch) error: %v", err)
	}

	events := decodeLifecycleEvents(t, &output)
	if len(events) != 5 {
		t.Fatalf("compiler log events = %#v, want 5", events)
	}

	checks := []map[string]any{
		{"message": "compiled ADK resource node", "node_role": "resource", "id": "review router", "kind": "Router", "ref": "workflows/review"},
		{"message": "compiled ADK resource node", "node_role": "resource", "id": "scripts/check", "kind": "Script"},
		{"message": "compiled ADK helper node", "node_role": "terminal"},
		{"message": "compiled ADK helper node", "node_role": "router_dispatch", "owner_id": "review router"},
		{"message": "compiled ADK helper node", "node_role": "router_branch", "owner_id": "review router", "child_id": "reviewer"},
	}

	for index, expected := range checks {
		event := events[index]
		if event["level"] != "debug" {
			t.Errorf("event %d level = %#v, want debug", index, event["level"])
		}

		if _, ok := event["adk_node"].(string); !ok {
			t.Errorf("event %d adk_node = %#v, want string", index, event["adk_node"])
		}

		for field, want := range expected {
			if event[field] != want {
				t.Errorf("event %d field %s = %#v, want %#v; event=%#v", index, field, event[field], want, event)
			}
		}

		for _, forbidden := range []string{"prompt", "input", "route", "state", "artifact", "output", "credential", "authorization"} {
			if _, ok := event[forbidden]; ok {
				t.Errorf("event %d contains forbidden field %q: %#v", index, forbidden, event)
			}
		}
	}

	if _, ok := events[1]["ref"]; ok {
		t.Errorf("resource with matching effective and resource IDs contains ref: %#v", events[1])
	}

	if strings.Contains(output.String(), secret) {
		t.Errorf("compiler logs contain authored payload %q: %s", secret, output.String())
	}
}

func TestADKCompilerSuppressesIdentityLogsAboveDebug(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer

	compiler := &adkCompiler{logger: zerolog.New(&output).Level(zerolog.InfoLevel)}
	if _, err := compiler.resourceName(&registry.ResolvedNode{
		EffectiveID: "roles/worker",
		ResourceID:  "roles/worker",
		Kind:        agent.RoleKind,
	}); err != nil {
		t.Fatalf("resourceName() error: %v", err)
	}

	if output.Len() != 0 {
		t.Errorf("Info-level compiler logs = %q, want empty", output.String())
	}
}

func TestADKCompilerSelectsOnlyNativeLeafExecutors(t *testing.T) {
	t.Parallel()

	compiler := &adkCompiler{run: &runState{}}
	for _, kind := range []agent.Kind{agent.RoleKind, agent.ScriptKind, agent.HumanKind, agent.TypeSafeJevKind, agent.OpenRouterDecisionKind} {
		if execute, ok := compiler.nativeLeafExecutor(kind); !ok || execute == nil {
			t.Errorf("nativeLeafExecutor(%q) = (%v, %t), want executor", kind, execute, ok)
		}
	}

	for _, kind := range []agent.Kind{agent.SequentialKind, agent.LoopKind, agent.RouterKind, "Unknown"} {
		if execute, ok := compiler.nativeLeafExecutor(kind); ok || execute != nil {
			t.Errorf("nativeLeafExecutor(%q) = (%v, %t), want no executor", kind, execute, ok)
		}
	}
}

func TestADKCompileErrorIncludesResourceContextAndPreservesCause(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("graph sentinel")
	node := &registry.ResolvedNode{
		EffectiveID: "reviewer",
		ResourceID:  "roles/reviewer",
		Kind:        agent.RoleKind,
	}

	err := wrapADKCompileError(node, wantErr)
	if !errors.Is(err, wantErr) {
		t.Fatalf("wrapADKCompileError() error = %v, want errors.Is(_, sentinel)", err)
	}

	for _, fragment := range []string{`compile Role "reviewer"`, `resource "roles/reviewer"`} {
		if !strings.Contains(err.Error(), fragment) {
			t.Errorf("wrapADKCompileError() error = %q, want fragment %q", err, fragment)
		}
	}

	compiler := &adkCompiler{run: &runState{}, logger: zerolog.Nop()}
	unsupported := &registry.ResolvedNode{EffectiveID: "judge", ResourceID: "judges/main", Kind: "Unknown"}

	_, err = compiler.compile(unsupported)
	if err == nil || !strings.Contains(err.Error(), `unsupported kind "Unknown"`) || !strings.Contains(err.Error(), `compile Unknown "judge"`) {
		t.Fatalf("compile(unsupported) error = %v, want contextual unsupported-kind error", err)
	}
}

func TestRunnerPreservesCalleeErrorChainAcrossADK(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("provider start sentinel")
	root := resolvedRoot(t, roleResource(t, "roles/worker", false, nil, "{{ .Input }}"))

	_, err := (Runner{
		Root:    root,
		Factory: errorFactory{err: wantErr},
	}).Run(context.Background(), "task")
	if !errors.Is(err, wantErr) {
		t.Fatalf("Runner.Run() error = %v, want errors.Is(_, sentinel)", err)
	}
}

func TestRunnerPreservesCancellationAcrossADK(t *testing.T) {
	t.Parallel()

	root := resolvedRoot(t, roleResource(t, "roles/worker", false, nil, "{{ .Input }}"))
	process := &scriptedProcess{
		visits:       map[string][][]string{"roles/worker": {{"done"}}},
		turnStarted:  make(chan struct{}, 1),
		releaseTurns: make(chan struct{}),
	}

	ctx, cancel := context.WithCancel(context.Background())

	var logs bytes.Buffer

	ctx = zerolog.New(&logs).WithContext(ctx)

	type runResult struct {
		artifact string
		err      error
	}

	done := make(chan runResult, 1)

	go func() {
		artifact, err := (Runner{
			Root:    root,
			Factory: &scriptedFactory{process: process},
		}).Run(ctx, "task")
		done <- runResult{artifact: artifact, err: err}
	}()

	<-process.turnStarted
	cancel()

	got := <-done
	if got.artifact != "" {
		t.Errorf("Runner.Run() artifact = %q, want empty", got.artifact)
	}

	if !errors.Is(got.err, context.Canceled) {
		t.Fatalf("Runner.Run() error = %v, want errors.Is(_, context.Canceled)", got.err)
	}

	if process.closes != 1 {
		t.Errorf("provider closes = %d, want 1", process.closes)
	}

	events := decodeLifecycleEvents(t, &logs)
	if len(events) == 0 {
		t.Fatal("lifecycle events are empty")
	}

	if gotMessage := events[len(events)-1]["message"]; gotMessage != "agent finished" {
		t.Errorf("last lifecycle message = %#v, want agent finished", gotMessage)
	}
}

type errorFactory struct {
	err error
}

func (f errorFactory) Start(context.Context, runtime.Provider) (runtime.ProviderProcess, error) {
	return nil, f.err
}
