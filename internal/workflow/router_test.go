package workflow

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/baldaworks/callee/internal/agent"
	"github.com/baldaworks/callee/internal/registry"
	"github.com/rs/zerolog"
)

func TestRunnerRouterSelectsNamedRouteOverDefault(t *testing.T) {
	t.Parallel()

	root := resolvedRoot(t,
		roleResource(t, "roles/bug", false, nil, "Bug: {{ .Input }}"),
		roleResource(t, "roles/default", false, nil, "Default: {{ .Input }}"),
		routerResource(t, "workflows/router", `  {{ .State.route }}  `, []agent.Child{
			{Ref: "roles/default", Alias: "fallback", Default: true},
			{Ref: "roles/bug", Alias: "bug", Route: "bug"},
		}, "payload={{ .Input }}", "router={{ .Output }}"),
	)
	root.Resource.Spec.State = map[string]any{"route": "bug"}

	process := &scriptedProcess{visits: map[string][][]string{
		"roles/bug":     {{"handled"}},
		"roles/default": {{"unexpected"}},
	}}

	got, err := (Runner{Root: root, Factory: &scriptedFactory{process: process}}).Run(context.Background(), "ticket")
	if err != nil {
		t.Fatalf("Runner.Run() error: %v", err)
	}

	if got != "router=handled" {
		t.Errorf("Runner.Run() = %q, want router=handled", got)
	}

	if prompts := process.prompts["roles/bug"]; len(prompts) != 1 || !strings.Contains(prompts[0], "Bug: payload=ticket") {
		t.Errorf("bug prompts = %#v, want routed payload", prompts)
	}

	if visits := process.visits["roles/default"]; len(visits) != 1 {
		t.Errorf("default remaining visits = %d, want 1", len(visits))
	}
}

func TestRunnerRouterSelectsDefaultForBlankAndUnknownRoutes(t *testing.T) {
	t.Parallel()

	for _, route := range []string{"   ", "other", "Named"} {
		t.Run(fmt.Sprintf("route_%q", route), func(t *testing.T) {
			t.Parallel()

			root := resolvedRoot(t,
				roleResource(t, "roles/named", false, nil, "Named: {{ .Input }}"),
				roleResource(t, "roles/default", false, nil, "Default: {{ .Input }}"),
				routerResource(t, "workflows/router", `{{ .State.route }}`, []agent.Child{
					{Ref: "roles/named", Alias: "named", Route: "named"},
					{Ref: "roles/default", Alias: "fallback", Default: true},
				}, "payload={{ .Input }}", ""),
			)
			root.Resource.Spec.State = map[string]any{"route": route}
			process := &scriptedProcess{visits: map[string][][]string{
				"roles/named":   {{"unexpected"}},
				"roles/default": {{"fallback"}},
			}}

			got, err := (Runner{Root: root, Factory: &scriptedFactory{process: process}}).Run(context.Background(), "ticket")
			if err != nil {
				t.Fatalf("Runner.Run() error: %v", err)
			}

			if got != "fallback" {
				t.Errorf("Runner.Run() = %q, want fallback", got)
			}

			if prompts := process.prompts["roles/default"]; len(prompts) != 1 || !strings.Contains(prompts[0], "Default: payload=ticket") {
				t.Errorf("default prompts = %#v, want routed payload", prompts)
			}

			if visits := process.visits["roles/named"]; len(visits) != 1 {
				t.Errorf("named remaining visits = %d, want 1", len(visits))
			}
		})
	}
}

func TestRunnerRouterFailsClosedBeforeChildActivity(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		route     string
		wantError string
	}{
		{name: "blank", route: "   ", wantError: "rendered a blank route"},
		{name: "unknown", route: "unknown", wantError: `available routes: "alpha", "zeta"`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			root := resolvedRoot(t,
				roleResource(t, "roles/alpha", false, nil, "{{ .Input }}"),
				roleResource(t, "roles/zeta", false, nil, "{{ .Input }}"),
				routerResource(t, "workflows/router", `{{ .State.route }}`, []agent.Child{
					{Ref: "roles/zeta", Alias: "zeta", Route: "zeta"},
					{Ref: "roles/alpha", Alias: "alpha", Route: "alpha"},
				}, "{{ .Input }}", ""),
			)
			root.Resource.Spec.State = map[string]any{"route": test.route}
			factory := &scriptedFactory{process: &scriptedProcess{}}

			_, err := (Runner{Root: root, Factory: factory}).Run(context.Background(), "ticket")
			if err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("Runner.Run() error = %v, want containing %q", err, test.wantError)
			}

			if factory.starts != 0 {
				t.Errorf("provider starts = %d, want 0", factory.starts)
			}
		})
	}
}

func TestRunnerRouterRouteTemplateFailureBypassesDefault(t *testing.T) {
	t.Parallel()

	root := resolvedRoot(t,
		roleResource(t, "roles/default", false, nil, "{{ .Input }}"),
		routerResource(t, "workflows/router", `{{ fail "route exploded" }}`, []agent.Child{
			{Ref: "roles/default", Alias: "fallback", Default: true},
		}, "{{ .Input }}", ""),
	)
	factory := &scriptedFactory{process: &scriptedProcess{}}

	_, err := (Runner{Root: root, Factory: factory}).Run(context.Background(), "ticket")
	if err == nil || !strings.Contains(err.Error(), "route exploded") {
		t.Fatalf("Runner.Run() error = %v, want route template failure", err)
	}

	if factory.starts != 0 {
		t.Errorf("provider starts = %d, want 0", factory.starts)
	}
}

func TestRunnerRouterChildInputOverridesPayload(t *testing.T) {
	t.Parallel()

	root := resolvedRoot(t,
		roleResource(t, "roles/worker", false, nil, "Worker: {{ .Input }}"),
		routerResource(t, "workflows/router", "worker", []agent.Child{
			{
				Ref:   "roles/worker",
				Alias: "worker",
				Route: "worker",
				Input: `override={{ .Input }} marker={{ .State.marker }}`,
			},
		}, "payload={{ .Input }}", ""),
	)
	root.Resource.Spec.State = map[string]any{"marker": "set"}
	process := &scriptedProcess{visits: map[string][][]string{"roles/worker": {{"done"}}}}

	_, err := (Runner{Root: root, Factory: &scriptedFactory{process: process}}).Run(context.Background(), "ticket")
	if err != nil {
		t.Fatalf("Runner.Run() error: %v", err)
	}

	if prompt := process.prompts["roles/worker"][0]; !strings.Contains(prompt, "Worker: override=payload=ticket marker=set") {
		t.Errorf("worker prompt = %q, want child input override", prompt)
	}
}

func TestRunnerRouterDoesNotVisitUnselectedSubtree(t *testing.T) {
	t.Parallel()

	marker := filepath.Join(t.TempDir(), "unselected-script-ran")
	unselectedScript := scriptResource(t, "scripts/unselected", fmt.Sprintf("touch %q", marker))
	unselectedHuman := humanResource(t, "humans/unselected", "answer", "should not prompt")
	unselectedRole := roleResource(t, "roles/unselected", false, nil, "{{ .Input }}")
	unselected := compositeResource(t, "workflows/unselected", agent.SequentialKind, []agent.Child{
		{Ref: "roles/unselected", Alias: "unselected_role"},
		{Ref: "scripts/unselected", Alias: "unselected_script"},
		{Ref: "humans/unselected", Alias: "unselected_human"},
	}, 0, "{{ .Input }}", "")
	selectedBody := `{{ .Input }} {{ if or (index .State "unselected") (index .State.outputs "unselected") ` +
		`(index .State.outputs "unselected_role") (index .State.outputs "unselected_script") ` +
		`(index .State.outputs "unselected_human") }}dirty{{ else }}clean{{ end }}`

	root := resolvedRoot(t,
		unselectedRole,
		unselectedScript,
		unselectedHuman,
		unselected,
		roleResource(t, "roles/selected", false, nil, selectedBody),
		routerResource(t, "workflows/router", "selected", []agent.Child{
			{Ref: "workflows/unselected", Alias: "unselected", Route: "other", State: map[string]any{"unselected": true}},
			{Ref: "roles/selected", Alias: "selected", Route: "selected"},
		}, "payload", ""),
	)
	process := &scriptedProcess{visits: map[string][][]string{
		"roles/unselected": {{"unexpected"}},
		"roles/selected":   {{"done"}},
	}}
	interactor := &scriptedInteractor{answers: []string{"unexpected"}}

	var logs bytes.Buffer

	ctx := zerolog.New(&logs).WithContext(context.Background())

	_, err := (Runner{
		Root:       root,
		Factory:    &scriptedFactory{process: process},
		Interactor: interactor,
	}).Run(ctx, "ticket")
	if err != nil {
		t.Fatalf("Runner.Run() error: %v", err)
	}

	if prompt := process.prompts["roles/selected"][0]; !strings.Contains(prompt, "payload clean") {
		t.Errorf("selected prompt = %q, want clean state", prompt)
	}

	if visits := process.visits["roles/unselected"]; len(visits) != 1 {
		t.Errorf("unselected Role remaining visits = %d, want 1", len(visits))
	}

	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Errorf("unselected Script marker stat error = %v, want not exist", err)
	}

	if len(interactor.labels) != 0 || len(interactor.displayed) != 0 {
		t.Errorf("unselected Human interactions = labels %#v displayed %#v, want none", interactor.labels, interactor.displayed)
	}

	for _, event := range decodeLifecycleEvents(t, &logs) {
		id, _ := event["id"].(string)
		if strings.HasPrefix(id, "unselected") {
			t.Errorf("unselected lifecycle event = %#v", event)
		}
	}
}

func TestRunnerRouterSelectedFailureDoesNotUseDefault(t *testing.T) {
	t.Parallel()

	root := resolvedRoot(t,
		scriptResource(t, "scripts/fail", "exit 7"),
		roleResource(t, "roles/default", false, nil, "{{ .Input }}"),
		routerResource(t, "workflows/router", "fail", []agent.Child{
			{Ref: "scripts/fail", Alias: "fail", Route: "fail"},
			{Ref: "roles/default", Alias: "fallback", Default: true},
		}, "payload", ""),
	)
	factory := &scriptedFactory{process: &scriptedProcess{}}

	_, err := (Runner{Root: root, Factory: factory}).Run(context.Background(), "ticket")
	if err == nil || !strings.Contains(err.Error(), `agent "fail" failed`) {
		t.Fatalf("Runner.Run() error = %v, want selected Script failure", err)
	}

	if factory.starts != 0 {
		t.Errorf("default provider starts = %d, want 0", factory.starts)
	}
}

func TestRunnerRouterRenderingFailuresDoNotUseDefault(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name  string
		body  string
		input string
	}{
		{name: "body", body: `{{ fail "body exploded" }}`},
		{name: "child_input", body: "payload", input: `{{ fail "input exploded" }}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			root := resolvedRoot(t,
				roleResource(t, "roles/selected", false, nil, "{{ .Input }}"),
				roleResource(t, "roles/default", false, nil, "{{ .Input }}"),
				routerResource(t, "workflows/router", "selected", []agent.Child{
					{Ref: "roles/selected", Alias: "selected", Route: "selected", Input: test.input},
					{Ref: "roles/default", Alias: "fallback", Default: true},
				}, test.body, ""),
			)
			factory := &scriptedFactory{process: &scriptedProcess{}}

			_, err := (Runner{Root: root, Factory: factory}).Run(context.Background(), "ticket")
			if err == nil || !strings.Contains(err.Error(), "exploded") {
				t.Fatalf("Runner.Run() error = %v, want rendering failure", err)
			}

			if factory.starts != 0 {
				t.Errorf("provider starts = %d, want 0", factory.starts)
			}
		})
	}
}

func TestRunnerRouterRejectsBlankFinalOutput(t *testing.T) {
	t.Parallel()

	for _, useDefault := range []bool{false, true} {
		t.Run(fmt.Sprintf("default_%t", useDefault), func(t *testing.T) {
			t.Parallel()

			route := "selected"
			child := agent.Child{Ref: "roles/worker", Alias: "worker", Route: "selected"}

			if useDefault {
				route = "unknown"
				child.Route = ""
				child.Default = true
			}

			root := resolvedRoot(t,
				roleResource(t, "roles/worker", false, nil, "{{ .Input }}"),
				routerResource(t, "workflows/router", route, []agent.Child{child}, "payload", "   "),
			)
			process := &scriptedProcess{visits: map[string][][]string{"roles/worker": {{"done"}}}}

			_, err := (Runner{Root: root, Factory: &scriptedFactory{process: process}}).Run(context.Background(), "ticket")
			if err == nil || !strings.Contains(err.Error(), `agent "workflows/router" returned an empty artifact`) {
				t.Fatalf("Runner.Run() error = %v, want blank Router output failure", err)
			}
		})
	}
}

func TestRunnerRouterPreservesSelectedProviderErrorChain(t *testing.T) {
	t.Parallel()

	for _, useDefault := range []bool{false, true} {
		t.Run(fmt.Sprintf("default_%t", useDefault), func(t *testing.T) {
			t.Parallel()

			route := "selected"
			child := agent.Child{Ref: "roles/worker", Alias: "worker", Route: "selected"}

			if useDefault {
				route = "unknown"
				child.Route = ""
				child.Default = true
			}

			root := resolvedRoot(t,
				roleResource(t, "roles/worker", false, nil, "{{ .Input }}"),
				routerResource(t, "workflows/router", route, []agent.Child{child}, "payload", ""),
			)
			wantErr := errors.New("router provider sentinel")

			_, err := (Runner{Root: root, Factory: errorFactory{err: wantErr}}).Run(context.Background(), "ticket")
			if !errors.Is(err, wantErr) {
				t.Fatalf("Runner.Run() error = %v, want errors.Is(_, sentinel)", err)
			}
		})
	}
}

func TestRunnerRouterPreservesCancellationAndCleanup(t *testing.T) {
	t.Parallel()

	for _, useDefault := range []bool{false, true} {
		t.Run(fmt.Sprintf("default_%t", useDefault), func(t *testing.T) {
			t.Parallel()

			route := "selected"
			child := agent.Child{Ref: "roles/worker", Alias: "worker", Route: "selected"}

			if useDefault {
				route = "unknown"
				child.Route = ""
				child.Default = true
			}

			root := resolvedRoot(t,
				roleResource(t, "roles/worker", false, nil, "{{ .Input }}"),
				routerResource(t, "workflows/router", route, []agent.Child{child}, "payload", ""),
			)
			process := &scriptedProcess{
				visits:       map[string][][]string{"roles/worker": {{"done"}}},
				turnStarted:  make(chan struct{}, 1),
				releaseTurns: make(chan struct{}),
			}
			ctx, cancel := context.WithCancel(context.Background())

			var logs bytes.Buffer

			ctx = zerolog.New(&logs).WithContext(ctx)

			done := make(chan error, 1)

			go func() {
				_, err := (Runner{Root: root, Factory: &scriptedFactory{process: process}}).Run(ctx, "ticket")
				done <- err
			}()

			<-process.turnStarted
			cancel()

			err := <-done
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("Runner.Run() error = %v, want errors.Is(_, context.Canceled)", err)
			}

			if process.closes != 1 {
				t.Errorf("provider closes = %d, want 1", process.closes)
			}

			finished := map[string]bool{}

			for _, event := range decodeLifecycleEvents(t, &logs) {
				if event["message"] == "agent finished" {
					id, _ := event["id"].(string)
					finished[id] = true
				}
			}

			for _, id := range []string{"worker", "workflows/router"} {
				if !finished[id] {
					t.Errorf("missing final lifecycle event for %q", id)
				}
			}
		})
	}
}

func TestRunnerRouterEscalationCompletesNearestLoop(t *testing.T) {
	t.Parallel()

	for _, defaultRoute := range []bool{false, true} {
		t.Run(fmt.Sprintf("default_%t", defaultRoute), func(t *testing.T) {
			t.Parallel()

			route := "approved"

			child := agent.Child{Ref: "roles/gate", Alias: "gate", Route: route, CanEscalate: true}

			if defaultRoute {
				route = "unknown"
				child.Route = ""
				child.Default = true
			}

			root := resolvedRoot(t,
				roleResource(t, "roles/gate", false, nil, "{{ .Input }}"),
				routerResource(t, "workflows/router", route, []agent.Child{child}, "payload", ""),
				compositeResource(t, "workflows/loop", agent.LoopKind, []agent.Child{
					{Ref: "workflows/router", Alias: "router", CanEscalate: true},
				}, 3, "{{ .Input }}", "loop={{ .State.outputs.router }}"),
			)
			process := &scriptedProcess{visits: map[string][][]string{
				"roles/gate": {{"approved\n\n" + controlEscalate}},
			}}

			got, err := (Runner{Root: root, Factory: &scriptedFactory{process: process}}).Run(context.Background(), "ticket")
			if err != nil {
				t.Fatalf("Runner.Run() error: %v", err)
			}

			if got != "loop=approved" {
				t.Errorf("Runner.Run() = %q, want loop=approved", got)
			}
		})
	}
}

func TestRunnerRouterRejectsUnauthorizedEscalationWithoutFallback(t *testing.T) {
	t.Parallel()

	for _, useDefault := range []bool{false, true} {
		t.Run(fmt.Sprintf("default_%t", useDefault), func(t *testing.T) {
			t.Parallel()

			route := "selected"
			child := agent.Child{Ref: "roles/gate", Alias: "gate", Route: "selected"}

			if useDefault {
				route = "unknown"
				child.Route = ""
				child.Default = true
			}

			root := resolvedRoot(t,
				roleResource(t, "roles/gate", false, nil, "{{ .Input }}"),
				roleResource(t, "roles/fallback", false, nil, "{{ .Input }}"),
				routerResource(t, "workflows/router", route, []agent.Child{
					child,
					{Ref: "roles/fallback", Alias: "fallback", Route: "fallback"},
				}, "payload", ""),
				compositeResource(t, "workflows/loop", agent.LoopKind, []agent.Child{
					{Ref: "workflows/router", Alias: "router", CanEscalate: true},
				}, 3, "{{ .Input }}", ""),
			)
			process := &scriptedProcess{visits: map[string][][]string{
				"roles/gate":     {{"stop\n\n" + controlEscalate}},
				"roles/fallback": {{"unexpected"}},
			}}

			_, err := (Runner{Root: root, Factory: &scriptedFactory{process: process}}).Run(context.Background(), "ticket")
			if err == nil || !strings.Contains(err.Error(), `agent "gate" (resource "roles/gate", path "workflows/loop -> router -> gate") attempted unauthorized escalation`) {
				t.Fatalf("Runner.Run() error = %v, want unauthorized escalation failure", err)
			}

			if visits := process.visits["roles/fallback"]; len(visits) != 1 {
				t.Errorf("fallback remaining visits = %d, want 1", len(visits))
			}
		})
	}
}

func TestRunnerRouterPromotesOutputForFollowingSequentialChild(t *testing.T) {
	t.Parallel()

	root := resolvedRoot(t,
		roleResource(t, "roles/worker", false, nil, "{{ .Input }}"),
		roleResource(t, "roles/reader", false, nil, `{{ .Input }} router={{ .State.outputs.router }}`),
		routerResource(t, "workflows/router", "worker", []agent.Child{
			{Ref: "roles/worker", Alias: "worker", Route: "worker"},
		}, "payload", "wrapped={{ .Output }}"),
		compositeResource(t, "workflows/pipeline", agent.SequentialKind, []agent.Child{
			{Ref: "workflows/router", Alias: "router"},
			{Ref: "roles/reader", Alias: "reader"},
		}, 0, "{{ .Input }}", ""),
	)
	process := &scriptedProcess{visits: map[string][][]string{
		"roles/worker": {{"handled"}},
		"roles/reader": {{"observed"}},
	}}

	got, err := (Runner{Root: root, Factory: &scriptedFactory{process: process}}).Run(context.Background(), "ticket")
	if err != nil {
		t.Fatalf("Runner.Run() error: %v", err)
	}

	if got != "observed" {
		t.Errorf("Runner.Run() = %q, want observed", got)
	}

	if prompt := process.prompts["roles/reader"][0]; !strings.Contains(prompt, "wrapped=handled router=wrapped=handled") {
		t.Errorf("reader prompt = %q, want Router output and promoted state", prompt)
	}
}

func TestRunnerRouterComposesNestedRouter(t *testing.T) {
	t.Parallel()

	root := resolvedRoot(t,
		roleResource(t, "roles/worker", false, nil, "{{ .Input }}"),
		routerResource(t, "workflows/inner", "inner", []agent.Child{
			{Ref: "roles/worker", Alias: "worker", Route: "inner"},
		}, "inner={{ .Input }}", ""),
		routerResource(t, "workflows/outer", "outer", []agent.Child{
			{Ref: "workflows/inner", Alias: "inner_router", Route: "outer"},
		}, "outer={{ .Input }}", ""),
	)
	process := &scriptedProcess{visits: map[string][][]string{"roles/worker": {{"done"}}}}

	got, err := (Runner{Root: root, Factory: &scriptedFactory{process: process}}).Run(context.Background(), "ticket")
	if err != nil {
		t.Fatalf("Runner.Run() error: %v", err)
	}

	if got != "done" {
		t.Errorf("Runner.Run() = %q, want done", got)
	}

	if prompt := process.prompts["roles/worker"][0]; !strings.Contains(prompt, "inner=outer=ticket") {
		t.Errorf("worker prompt = %q, want nested Router payload", prompt)
	}
}

func TestUnmatchedRouteErrorBoundsRenderedKey(t *testing.T) {
	t.Parallel()

	children := make([]*registry.ResolvedNode, 0, 20)
	for index := range 20 {
		children = append(children, &registry.ResolvedNode{
			Edge: agent.Child{Route: fmt.Sprintf("route-%02d-%s", index, strings.Repeat("x", 200))},
		})
	}

	node := &registry.ResolvedNode{
		EffectiveID: "router",
		Children:    children,
	}
	route := strings.Repeat("sensitive", 40)

	err := unmatchedRouteError(node, route)
	if strings.Contains(err.Error(), route) {
		t.Fatalf("unmatchedRouteError() leaked full rendered route: %v", err)
	}

	if !strings.Contains(err.Error(), "…") {
		t.Fatalf("unmatchedRouteError() = %v, want truncation marker", err)
	}

	if !strings.Contains(err.Error(), "(12 more)") {
		t.Fatalf("unmatchedRouteError() = %v, want bounded available-route count", err)
	}

	if len(err.Error()) > 2_000 {
		t.Fatalf("unmatchedRouteError() length = %d, want bounded diagnostics", len(err.Error()))
	}
}

func TestRunnerRouterBoundsMatchedRouteLifecycleField(t *testing.T) {
	t.Parallel()

	route := strings.Repeat("route", 80)
	root := resolvedRoot(t,
		roleResource(t, "roles/worker", false, nil, "{{ .Input }}"),
		routerResource(t, "workflows/router", route, []agent.Child{
			{Ref: "roles/worker", Alias: "worker", Route: route},
		}, "payload", ""),
	)
	process := &scriptedProcess{visits: map[string][][]string{"roles/worker": {{"done"}}}}

	var logs bytes.Buffer

	ctx := zerolog.New(&logs).WithContext(context.Background())

	if _, err := (Runner{Root: root, Factory: &scriptedFactory{process: process}}).Run(ctx, "ticket"); err != nil {
		t.Fatalf("Runner.Run() error: %v", err)
	}

	for _, event := range decodeLifecycleEvents(t, &logs) {
		if event["message"] != "selected router child" {
			continue
		}

		got, _ := event["route"].(string)
		if len([]rune(got)) > maxRouteDiagnosticRunes+1 {
			t.Errorf("route lifecycle field length = %d runes, want at most %d plus ellipsis", len([]rune(got)), maxRouteDiagnosticRunes)
		}

		if event["route_truncated"] != true {
			t.Errorf("route_truncated = %#v, want true", event["route_truncated"])
		}

		return
	}

	t.Fatal("selection lifecycle event not found")
}

func TestRunnerRouterLogsNamedAndDefaultSelection(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name        string
		route       string
		wantField   string
		wantValue   any
		wantNoField string
	}{
		{name: "named", route: "named", wantField: "route", wantValue: "named", wantNoField: "default"},
		{name: "default", route: "unknown", wantField: "default", wantValue: true, wantNoField: "route"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			children := []agent.Child{{Ref: "roles/worker", Alias: "worker", Route: "named"}}
			if test.name == "default" {
				children = append(children, agent.Child{Ref: "roles/default", Alias: "fallback", Default: true})
			}

			root := resolvedRoot(t,
				roleResource(t, "roles/worker", false, nil, "{{ .Input }}"),
				roleResource(t, "roles/default", false, nil, "{{ .Input }}"),
				routerResource(t, "workflows/router", test.route, children, "payload", ""),
			)
			process := &scriptedProcess{visits: map[string][][]string{
				"roles/worker":  {{"done"}},
				"roles/default": {{"done"}},
			}}

			var logs bytes.Buffer

			ctx := zerolog.New(&logs).WithContext(context.Background())

			if _, err := (Runner{Root: root, Factory: &scriptedFactory{process: process}}).Run(ctx, "ticket"); err != nil {
				t.Fatalf("Runner.Run() error: %v", err)
			}

			var selection map[string]any

			for _, event := range decodeLifecycleEvents(t, &logs) {
				if event["message"] == "selected router child" {
					selection = event
				}
			}

			if selection == nil {
				t.Fatal("selection lifecycle event not found")
			}

			if got := selection[test.wantField]; got != test.wantValue {
				t.Errorf("selection[%s] = %#v, want %#v", test.wantField, got, test.wantValue)
			}

			if _, ok := selection[test.wantNoField]; ok {
				t.Errorf("selection unexpectedly contains %q: %#v", test.wantNoField, selection)
			}
		})
	}
}

func routerResource(t *testing.T, id, route string, children []agent.Child, body, output string) agent.Resource {
	t.Helper()

	resource := agent.Resource{
		APIVersion: agent.APIVersion,
		Kind:       agent.RouterKind,
		ID:         id,
		Source:     id + ".yaml",
		Spec: agent.Spec{
			Description: id,
			Route:       route,
			Children:    children,
			Body:        body,
			Output:      output,
		},
	}
	if err := resource.Validate(); err != nil {
		t.Fatalf("router resource %q validation error: %v", id, err)
	}

	return resource
}
