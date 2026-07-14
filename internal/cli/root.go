// Package cli implements the Callee command-line surface.
package cli

import (
	"context"
	"flag"
	"fmt"
	"io"

	"github.com/baldaworks/callee/internal/registry"
	"github.com/baldaworks/callee/internal/runtime"
	"github.com/baldaworks/callee/internal/server"
)

const Version = "0.1.0"

// Run runs Callee and returns its process exit code.
func Run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) == 1 && args[0] == "--version" {
		fmt.Fprintln(stdout, Version)
		return 0
	}
	if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
		usage(stdout)
		return 0
	}
	if len(args) > 0 && args[0] == "mcp-server" {
		return runMCP(ctx, args[1:], stderr)
	}
	return runOneShot(ctx, args, stdout, stderr)
}

func load(rolesDir string) (*registry.Registry, error) {
	return registry.Load(registry.LoadOptions{RolesDir: rolesDir})
}
func usage(w io.Writer) {
	fmt.Fprintln(w, "Usage:\n  callee --role ROLE --prompt PROMPT [--roles-dir PATH]\n  callee mcp-server [--roles-dir PATH]\n  callee --help\n  callee --version")
}

func runOneShot(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("callee", flag.ContinueOnError)
	fs.SetOutput(stderr)
	roleID, prompt, rolesDir := fs.String("role", "", "role ID"), fs.String("prompt", "", "prompt"), fs.String("roles-dir", "", "roles directory")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *roleID == "" || *prompt == "" {
		fmt.Fprintln(stderr, "--role and --prompt are required")
		return 2
	}
	reg, err := load(*rolesDir)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	r, err := reg.Get(*roleID)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	rendered, err := r.Render(*prompt)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	manager := runtime.NewManager(runtime.NormaFactory{})
	defer manager.Close()
	_, content, err := manager.Start(ctx, r, rendered)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	fmt.Fprintln(stdout, content)
	return 0
}

func runMCP(ctx context.Context, args []string, stderr io.Writer) int {
	fs := flag.NewFlagSet("callee mcp-server", flag.ContinueOnError)
	fs.SetOutput(stderr)
	rolesDir := fs.String("roles-dir", "", "roles directory")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	reg, err := load(*rolesDir)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	manager := runtime.NewManager(runtime.NormaFactory{})
	defer manager.Close()
	if err := server.New(reg, manager).RunStdio(ctx, Version); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}
