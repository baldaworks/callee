package doctor

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/baldaworks/callee/internal/role"
)

type fakeChecker struct {
	called    []string
	errs      map[string]error
	deadlines []time.Time
}

func (f *fakeChecker) Check(ctx context.Context, r role.Role) error {
	f.called = append(f.called, r.ID)
	deadline, _ := ctx.Deadline()
	f.deadlines = append(f.deadlines, deadline)

	return f.errs[r.ID]
}

func TestRunChecksRolesInSortedOrder(t *testing.T) {
	checker := &fakeChecker{}

	var stdout bytes.Buffer

	err := Run(context.Background(), []role.Role{testRole("reviewer", "claude"), testRole("explorer", "codex")}, checker, time.Minute, &stdout)
	if err != nil {
		t.Fatal(err)
	}

	if got, want := strings.Join(checker.called, ","), "explorer,reviewer"; got != want {
		t.Fatalf("checked roles = %q, want %q", got, want)
	}

	if len(checker.deadlines) != 2 || checker.deadlines[0].IsZero() || checker.deadlines[1].IsZero() {
		t.Fatal("doctor checks must receive timeout contexts")
	}

	if got, want := stdout.String(), "role \"explorer\": ok\nrole \"reviewer\": ok\ncallee doctor: ok\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}

func TestRunReportsEveryFailedRole(t *testing.T) {
	checker := &fakeChecker{errs: map[string]error{
		"explorer": errors.New("explorer failed"),
		"reviewer": errors.New("reviewer failed"),
	}}

	err := Run(context.Background(), []role.Role{testRole("reviewer", "claude"), testRole("explorer", "codex")}, checker, time.Minute, &bytes.Buffer{})
	if err == nil {
		t.Fatal("Run() error = nil")
	}

	for _, want := range []string{"2 failing role(s)", "role \"explorer\": explorer failed", "role \"reviewer\": reviewer failed"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("Run() error = %q, want %q", err, want)
		}
	}

	if got, want := strings.Join(checker.called, ","), "explorer,reviewer"; got != want {
		t.Fatalf("checked roles = %q, want %q", got, want)
	}
}

func TestRunRejectsEmptyRolesAndInvalidTimeout(t *testing.T) {
	checker := &fakeChecker{}
	if err := Run(context.Background(), nil, checker, time.Minute, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "no roles found") {
		t.Fatalf("empty roles error = %v", err)
	}

	if err := Run(context.Background(), []role.Role{testRole("reviewer", "codex")}, checker, 0, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "greater than zero") {
		t.Fatalf("zero timeout error = %v", err)
	}
}

func TestRunSharesProviderChecks(t *testing.T) {
	checker := &fakeChecker{}

	roles := []role.Role{testRole("reviewer", "codex"), testRole("explorer", "codex")}
	if err := Run(context.Background(), roles, checker, time.Minute, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}

	if got, want := strings.Join(checker.called, ","), "explorer"; got != want {
		t.Fatalf("checked roles = %q, want %q", got, want)
	}
}

func testRole(id, kind string) role.Role {
	return role.Role{ID: id, Metadata: role.Metadata{API: role.CurrentAPI, Kind: role.RoleKind, Description: id, Provider: role.Provider{Type: kind}}, Template: "{{ prompt }}"}
}
