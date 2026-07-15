package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestSetupCommandInstallsPluginAndCreatesReviewer(t *testing.T) {
	tests := []struct {
		name         string
		target       string
		wantCommands [][]string
		wantType     string
	}{
		{
			name:   "codex",
			target: "codex",
			wantCommands: [][]string{
				{"codex", "plugin", "marketplace", "remove", "callee"},
				{"codex", "plugin", "marketplace", "add", "baldaworks/callee"},
				{"codex", "plugin", "add", "callee@callee"},
			},
			wantType: "  type: codex",
		},
		{
			name:   "claude",
			target: "claude",
			wantCommands: [][]string{
				{"claude", "plugin", "marketplace", "add", "baldaworks/callee"},
				{"claude", "plugin", "install", "callee@callee", "--scope", "project"},
			},
			wantType: "  type: claude",
		},
		{
			name:   "grok",
			target: "grok",
			wantCommands: [][]string{
				{"grok", "plugin", "marketplace", "add", "baldaworks/callee"},
				{"grok", "plugin", "install", "callee@callee", "--trust"},
			},
			wantType: "  type: grok",
		},
		{
			name:   "copilot",
			target: "copilot",
			wantCommands: [][]string{
				{"copilot", "plugin", "marketplace", "add", "baldaworks/callee"},
				{"copilot", "plugin", "install", "callee@callee"},
			},
			wantType: "  type: copilot",
		},
		{
			name:     "opencode",
			target:   "opencode",
			wantType: "  type: opencode",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Chdir(t.TempDir())

			original := runSetupCommand

			t.Cleanup(func() { runSetupCommand = original })

			var gotCommands [][]string

			runSetupCommand = func(_ context.Context, _ io.Writer, _ io.Writer, name string, args ...string) error {
				gotCommands = append(gotCommands, append([]string{name}, args...))

				return nil
			}

			cmd := NewRootCommand()

			var stdout bytes.Buffer
			cmd.SetOut(&stdout)
			cmd.SetArgs([]string{"setup", test.target})

			if err := cmd.Execute(); err != nil {
				t.Fatal(err)
			}

			if !reflect.DeepEqual(gotCommands, test.wantCommands) {
				t.Fatalf("commands = %#v, want %#v", gotCommands, test.wantCommands)
			}

			role, err := os.ReadFile(filepath.FromSlash(reviewerRolePath))
			if err != nil {
				t.Fatal(err)
			}

			for _, want := range []string{"api: callee.metalagman.dev", "kind: role", "provider:\n", test.wantType, "{{ prompt }}"} {
				if !strings.Contains(string(role), want) {
					t.Fatalf("reviewer role does not contain %q: %q", want, role)
				}
			}

			if !strings.Contains(stdout.String(), "Created "+reviewerRolePath) {
				t.Fatalf("stdout = %q", stdout.String())
			}
		})
	}
}

func TestPrepareCodexMarketplaceIgnoresMissingRegistration(t *testing.T) {
	original := runSetupCommand

	t.Cleanup(func() { runSetupCommand = original })

	runSetupCommand = func(_ context.Context, _ io.Writer, stderr io.Writer, _ string, _ ...string) error {
		_, _ = fmt.Fprintln(stderr, "Error: marketplace `callee` is not configured or installed")

		return errors.New("exit status 1")
	}

	var stderr bytes.Buffer
	if err := prepareCodexMarketplace(context.Background(), &stderr); err != nil {
		t.Fatal(err)
	}

	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

func TestPrepareCodexMarketplaceReportsUnexpectedFailure(t *testing.T) {
	original := runSetupCommand

	t.Cleanup(func() { runSetupCommand = original })

	runSetupCommand = func(_ context.Context, _ io.Writer, stderr io.Writer, _ string, _ ...string) error {
		_, _ = fmt.Fprintln(stderr, "permission denied")

		return errors.New("exit status 1")
	}

	var stderr bytes.Buffer

	err := prepareCodexMarketplace(context.Background(), &stderr)
	if err == nil || !strings.Contains(err.Error(), "remove existing Codex marketplace") {
		t.Fatalf("prepareCodexMarketplace() error = %v", err)
	}

	if !strings.Contains(stderr.String(), "permission denied") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestOpenCodeSetupInstallsSkillsAndCommands(t *testing.T) {
	t.Chdir(t.TempDir())

	cmd := NewRootCommand()
	cmd.SetArgs([]string{"setup", "opencode"})

	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	for _, asset := range openCodeAssetFiles {
		want, err := openCodeAssets.ReadFile(asset.source)
		if err != nil {
			t.Fatal(err)
		}

		got, err := os.ReadFile(filepath.FromSlash(asset.destination))
		if err != nil {
			t.Fatal(err)
		}

		if !bytes.Equal(got, want) {
			t.Errorf("%s differs from embedded asset %s", asset.destination, asset.source)
		}
	}
}

func TestOpenCodeSetupPreservesAssetsUnlessForced(t *testing.T) {
	t.Chdir(t.TempDir())

	path := filepath.FromSlash(".opencode/commands/callee.md")
	if err := os.MkdirAll(filepath.Dir(path), reviewerDirMode); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(path, []byte("custom"), reviewerFileMode); err != nil {
		t.Fatal(err)
	}

	result, err := writeOpenCodeIntegration(false)
	if err != nil {
		t.Fatal(err)
	}

	if len(result.created) != len(openCodeAssetFiles)-1 || len(result.unchanged) != 1 {
		t.Fatalf("first install result = %#v", result)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if string(got) != "custom" {
		t.Fatalf("custom command = %q", got)
	}

	result, err = writeOpenCodeIntegration(true)
	if err != nil {
		t.Fatal(err)
	}

	if len(result.created) != len(openCodeAssetFiles) || len(result.unchanged) != 0 {
		t.Fatalf("forced install result = %#v", result)
	}

	want, err := openCodeAssets.ReadFile("assets/opencode/commands/callee.md")
	if err != nil {
		t.Fatal(err)
	}

	got, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(got, want) {
		t.Fatalf("forced command = %q, want %q", got, want)
	}
}

func TestOpenCodeSkillAssetsMatchPluginSkills(t *testing.T) {
	for source, pluginPath := range map[string]string{
		"assets/opencode/skills/callee-run-role/SKILL.md":    filepath.Join("..", "..", "plugins", "callee", "prefixed-skills", "callee-run-role", "SKILL.md"),
		"assets/opencode/skills/callee-create-role/SKILL.md": filepath.Join("..", "..", "plugins", "callee", "prefixed-skills", "callee-create-role", "SKILL.md"),
	} {
		want, err := os.ReadFile(pluginPath)
		if err != nil {
			t.Fatal(err)
		}

		got, err := openCodeAssets.ReadFile(source)
		if err != nil {
			t.Fatal(err)
		}

		if !bytes.Equal(got, want) {
			t.Errorf("OpenCode asset %s differs from %s", source, pluginPath)
		}
	}
}

func TestOpenCodeCommandAssetsLoadTheMatchingSkill(t *testing.T) {
	for source, skill := range map[string]string{
		"assets/opencode/commands/callee.md":           "callee-run-role",
		"assets/opencode/commands/callee-promptkit.md": "callee-create-role",
	} {
		data, err := openCodeAssets.ReadFile(source)
		if err != nil {
			t.Fatal(err)
		}

		for _, want := range []string{"Load the `" + skill + "` skill", "$ARGUMENTS"} {
			if !strings.Contains(string(data), want) {
				t.Errorf("OpenCode command %s is missing %q", source, want)
			}
		}
	}
}

func TestSetupCommandLeavesExistingReviewerUntouched(t *testing.T) {
	t.Chdir(t.TempDir())

	path := filepath.FromSlash(reviewerRolePath)
	if err := os.MkdirAll(filepath.Dir(path), reviewerDirMode); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(path, []byte("existing"), reviewerFileMode); err != nil {
		t.Fatal(err)
	}

	original := runSetupCommand

	t.Cleanup(func() { runSetupCommand = original })

	runSetupCommand = func(context.Context, io.Writer, io.Writer, string, ...string) error { return nil }

	cmd := NewRootCommand()

	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetArgs([]string{"setup", "codex"})

	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	role, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if string(role) != "existing" {
		t.Fatalf("reviewer role = %q, want existing", role)
	}

	if !strings.Contains(stdout.String(), "already exists") {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestSetupCommandRejectsUnknownTarget(t *testing.T) {
	cmd := NewRootCommand()
	cmd.SetArgs([]string{"setup", "other"})

	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "unsupported setup target") {
		t.Fatalf("setup error = %v", err)
	}
}
