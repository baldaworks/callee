package registry

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func writeRole(t *testing.T, dir, name, description string) {
	t.Helper()

	p := filepath.Join(dir, name+".md")
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(p, []byte("---\ndescription: "+description+"\ntype: codex\n---\n{{ prompt }}"), 0644); err != nil {
		t.Fatal(err)
	}
}
func TestLoadDirectories(t *testing.T) {
	root := t.TempDir()
	user, project, explicit := filepath.Join(root, "user"), filepath.Join(root, "project"), filepath.Join(root, "explicit")
	writeRole(t, user, "reviewer", "user")
	writeRole(t, user, "code/explorer", "explorer")
	writeRole(t, project, "reviewer", "project")
	writeRole(t, explicit, "only", "only")

	r, err := Load(LoadOptions{UserDir: user, ProjectDir: project})
	if err != nil {
		t.Fatal(err)
	}

	if got := r.Roles()[1].Metadata.Description; got != "project" {
		t.Fatalf("override=%q", got)
	}

	if !reflect.DeepEqual(r.IDs(), []string{"code/explorer", "reviewer"}) {
		t.Fatal(r.IDs())
	}

	r, err = Load(LoadOptions{RolesDir: explicit, UserDir: user, ProjectDir: project})
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(r.IDs(), []string{"only"}) {
		t.Fatal(r.IDs())
	}

	if _, err = Load(LoadOptions{RolesDir: filepath.Join(root, "none")}); err != nil {
		t.Fatal(err)
	}
}
