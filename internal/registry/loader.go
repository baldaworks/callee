package registry

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/baldaworks/callee/internal/role"
)

// LoadOptions controls role discovery.
type LoadOptions struct {
	RolesDir   string
	UserDir    string
	ProjectDir string
	HomeDir    string
}

// Load discovers roles. Project roles override user roles.
func Load(opts LoadOptions) (*Registry, error) {
	if opts.RolesDir != "" {
		return loadDirectories([]string{opts.RolesDir})
	}

	if opts.HomeDir == "" {
		opts.HomeDir, _ = os.UserHomeDir()
	}

	if opts.UserDir == "" {
		base := os.Getenv("XDG_CONFIG_HOME")
		if base == "" {
			base = filepath.Join(opts.HomeDir, ".config")
		}

		opts.UserDir = filepath.Join(base, "callee", "roles")
	}

	if opts.ProjectDir == "" {
		opts.ProjectDir = filepath.Join(".callee", "roles")
	}

	return loadDirectories([]string{opts.UserDir, opts.ProjectDir})
}

func loadDirectories(dirs []string) (*Registry, error) {
	all := map[string]role.Role{}

	for _, dir := range dirs {
		loaded, err := loadDirectory(dir)
		if err != nil {
			return nil, err
		}

		for id, item := range loaded {
			all[id] = item
		}
	}

	items := make([]role.Role, 0, len(all))
	for _, item := range all {
		items = append(items, item)
	}

	return New(items)
}

func loadDirectory(dir string) (map[string]role.Role, error) {
	loaded := map[string]role.Role{}
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return loaded, nil
	} else if err != nil {
		return nil, fmt.Errorf("read roles directory %q: %w", dir, err)
	}

	err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if entry.IsDir() || !entry.Type().IsRegular() || filepath.Ext(entry.Name()) != ".md" {
			return nil
		}

		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}

		id := strings.TrimSuffix(filepath.ToSlash(rel), ".md")
		if _, exists := loaded[id]; exists {
			return fmt.Errorf("duplicate role %q in %q", id, dir)
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		item, err := role.Parse(id, data)
		if err != nil {
			return fmt.Errorf("parse %q: %w", path, err)
		}

		loaded[id] = item

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("load roles from %q: %w", dir, err)
	}

	return loaded, nil
}
