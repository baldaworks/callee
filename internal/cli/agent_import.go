package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	resource "github.com/baldaworks/callee/internal/agent"
	"github.com/baldaworks/callee/internal/registry"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

const defaultImportRemotePath = ".callee"

var cloneAgentImportRepository = cloneAgentImportRepositoryDefault

var githubImportRepositorySlugPattern = regexp.MustCompile(`^[A-Za-z0-9._-]+/[A-Za-z0-9._-]+$`)

type agentImportOptions struct {
	ref    string
	path   string
	prefix string
	force  bool
}

type importedAgentFile struct {
	id           string
	sourcePath   string
	relativePath string
	extension    string
	resource     resource.Resource
}

type renderedImportedAgentFile struct {
	resource        resource.Resource
	destination     string
	relativePath    string
	content         []byte
	overwriteTarget bool
}

type agentImportResult struct {
	created     []string
	overwritten []string
	unchanged   []string
}

func agentImportCommand() *cobra.Command {
	opts := &agentImportOptions{}

	cmd := &cobra.Command{
		Use:   "import <repo>",
		Short: "Import Callee agents from a remote git repository",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAgentImport(cmd, args[0], opts)
		},
	}
	cmd.Flags().StringVar(&opts.ref, "ref", "", "checkout this git ref after cloning")
	cmd.Flags().StringVar(&opts.path, "path", defaultImportRemotePath, "remote directory root containing Callee resources")
	cmd.Flags().StringVar(&opts.prefix, "prefix", "", "prepend this namespace to imported resource IDs")
	cmd.Flags().BoolVar(&opts.force, "force", false, "overwrite destination files selected by this import")

	return cmd
}

func runAgentImport(cmd *cobra.Command, repoURL string, opts *agentImportOptions) error {
	repoSpec := strings.TrimSpace(repoURL)
	if repoSpec == "" {
		return fmt.Errorf("repo URL must not be blank")
	}

	repoURL = normalizeAgentImportRepository(repoSpec)

	remotePath, err := cleanImportRelativeDirectory(opts.path)
	if err != nil {
		return fmt.Errorf("invalid --path: %w", err)
	}

	prefix, err := cleanImportPrefix(opts.prefix)
	if err != nil {
		return fmt.Errorf("invalid --prefix: %w", err)
	}

	checkoutDir, err := cloneAgentImportRepository(cmd.Context(), repoURL, strings.TrimSpace(opts.ref))
	if err != nil {
		return err
	}

	remoteRoot := filepath.Join(checkoutDir, remotePath)

	info, err := os.Stat(remoteRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("remote path %q does not exist in %q", remotePath, repoURL)
		}

		return fmt.Errorf("stat remote path %q in %q: %w", remotePath, repoURL, err)
	}

	if !info.IsDir() {
		return fmt.Errorf("remote path %q in %q is not a directory", remotePath, repoURL)
	}

	discovered, err := discoverImportedAgentFiles(remoteRoot)
	if err != nil {
		return err
	}

	if len(discovered) == 0 {
		return fmt.Errorf("no supported Callee agent files were found under remote path %q", remotePath)
	}

	importRoot, validationMode, err := importDestinationRoot(cmd)
	if err != nil {
		return err
	}

	rendered, result, err := prepareImportedAgentFiles(discovered, importRoot, prefix, opts.force)
	if err != nil {
		return err
	}

	stageRoot, err := os.MkdirTemp("", "callee-agent-import-stage-*")
	if err != nil {
		return fmt.Errorf("create staged import directory: %w", err)
	}
	defer os.RemoveAll(stageRoot)

	stageAgentsRoot, err := stageImportRoot(stageRoot, validationMode)
	if err != nil {
		return err
	}

	if err := copyAgentTree(importRoot, stageAgentsRoot); err != nil {
		return err
	}

	if err := applyImportedAgentFiles(stageAgentsRoot, importRoot, rendered); err != nil {
		return err
	}

	if err := validateImportedRegistry(validationMode, stageAgentsRoot); err != nil {
		return err
	}

	if err := applyImportedAgentFiles(importRoot, importRoot, rendered); err != nil {
		return err
	}

	return reportAgentImportResult(cmd.OutOrStdout(), result)
}

func normalizeAgentImportRepository(value string) string {
	if !githubImportRepositorySlugPattern.MatchString(value) {
		return value
	}

	parts := strings.Split(value, "/")
	if len(parts) != 2 || strings.HasSuffix(parts[1], ".git") {
		return value
	}

	for _, part := range parts {
		if part == "." || part == ".." {
			return value
		}
	}

	if strings.HasPrefix(value, "./") || strings.HasPrefix(value, "../") {
		return value
	}

	return "https://github.com/" + value + ".git"
}

func cloneAgentImportRepositoryDefault(ctx context.Context, repoURL, ref string) (string, error) {
	root, err := os.MkdirTemp("", "callee-agent-import-repo-*")
	if err != nil {
		return "", fmt.Errorf("create import checkout directory: %w", err)
	}

	checkoutDir := filepath.Join(root, "checkout")
	if err := runGitImportCommand(ctx, "clone", "--quiet", repoURL, checkoutDir); err != nil {
		os.RemoveAll(root)

		return "", err
	}

	if ref != "" {
		if err := runGitImportCommand(ctx, "-C", checkoutDir, "checkout", "--quiet", ref); err != nil {
			os.RemoveAll(root)

			return "", err
		}
	}

	return checkoutDir, nil
}

func runGitImportCommand(ctx context.Context, args ...string) error {
	command := exec.CommandContext(ctx, "git", args...)
	if output, err := command.CombinedOutput(); err != nil {
		message := strings.TrimSpace(string(output))

		if errors.Is(err, exec.ErrNotFound) {
			return fmt.Errorf("git is required to import remote agents: %w", err)
		}

		if message == "" {
			return fmt.Errorf("run git %s: %w", strings.Join(args, " "), err)
		}

		return fmt.Errorf("run git %s: %s", strings.Join(args, " "), message)
	}

	return nil
}

func discoverImportedAgentFiles(root string) ([]importedAgentFile, error) {
	byID := make(map[string]importedAgentFile)

	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			if entry != nil && entry.IsDir() {
				return filepath.SkipDir
			}

			return fmt.Errorf("visit import path %q: %w", path, walkErr)
		}

		if entry.IsDir() {
			return nil
		}

		if entry.Type()&os.ModeSymlink != 0 || !entry.Type().IsRegular() || !resource.SupportsFile(entry.Name()) {
			return nil
		}

		relative, err := filepath.Rel(root, path)
		if err != nil {
			return fmt.Errorf("resolve import path %q relative to %q: %w", path, root, err)
		}

		id, err := registry.ResourceID(relative)
		if err != nil {
			return fmt.Errorf("resource path %q: %w", path, err)
		}

		if previous, exists := byID[id]; exists {
			return fmt.Errorf("duplicate agent ID %q from %q and %q", id, previous.sourcePath, path)
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read imported agent %q: %w", path, err)
		}

		decoded, err := resource.Decode(id, path, data)
		if err != nil {
			return fmt.Errorf("parse imported agent %q: %w", path, err)
		}

		byID[id] = importedAgentFile{
			id:           id,
			sourcePath:   path,
			relativePath: relative,
			extension:    filepath.Ext(path),
			resource:     decoded,
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	imported := make([]importedAgentFile, 0, len(byID))
	for _, file := range byID {
		imported = append(imported, file)
	}

	sort.Slice(imported, func(i, j int) bool {
		if imported[i].id != imported[j].id {
			return imported[i].id < imported[j].id
		}

		return imported[i].sourcePath < imported[j].sourcePath
	})

	return imported, nil
}

func importDestinationRoot(cmd *cobra.Command) (root string, validationMode registry.AgentLoadOptions, err error) {
	if value, err := commandAgentRoot(cmd); err != nil {
		return "", registry.AgentLoadOptions{}, err
	} else if value != "" {
		clean := filepath.Clean(value)

		return clean, registry.AgentLoadOptions{ExclusiveDir: clean}, nil
	}

	return defaultProjectAgentDir, registry.AgentLoadOptions{ProjectDir: defaultProjectAgentDir}, nil
}

func prepareImportedAgentFiles(files []importedAgentFile, importRoot, prefix string, force bool) ([]renderedImportedAgentFile, agentImportResult, error) {
	rewrittenIDs := make(map[string]string, len(files))
	for _, file := range files {
		rewrittenIDs[file.id] = prefixedImportedID(file.id, prefix)
	}

	rendered := make([]renderedImportedAgentFile, 0, len(files))
	result := agentImportResult{}

	for _, file := range files {
		transformed := file.resource

		transformed.ID = rewrittenIDs[file.id]

		for index, child := range transformed.Spec.Children {
			if target, ok := rewrittenIDs[child.Ref]; ok {
				transformed.Spec.Children[index].Ref = target
			}
		}

		content, err := encodeImportedAgentFile(transformed, file.extension)
		if err != nil {
			return nil, agentImportResult{}, fmt.Errorf("encode imported agent %q: %w", transformed.ID, err)
		}

		destinationRelative := filepath.FromSlash(transformed.ID) + file.extension
		destination := filepath.Join(importRoot, destinationRelative)
		overwriteTarget := false

		if info, err := os.Stat(destination); err == nil {
			if info.IsDir() {
				return nil, agentImportResult{}, fmt.Errorf("destination %q is a directory", destination)
			}

			if !force {
				result.unchanged = append(result.unchanged, destination)

				continue
			}

			overwriteTarget = true

			result.overwritten = append(result.overwritten, destination)
		} else if !os.IsNotExist(err) {
			return nil, agentImportResult{}, fmt.Errorf("check destination %q: %w", destination, err)
		} else {
			result.created = append(result.created, destination)
		}

		rendered = append(rendered, renderedImportedAgentFile{
			resource:        transformed,
			destination:     destination,
			relativePath:    destinationRelative,
			content:         content,
			overwriteTarget: overwriteTarget,
		})
	}

	return rendered, result, nil
}

func prefixedImportedID(id, prefix string) string {
	if prefix == "" {
		return id
	}

	return prefix + "/" + id
}

func encodeImportedAgentFile(decoded resource.Resource, extension string) ([]byte, error) {
	switch extension {
	case ".md":
		return resource.EncodeMarkdown(decoded)
	case ".yaml", ".yml":
		if err := decoded.Validate(); err != nil {
			return nil, err
		}

		content, err := yaml.Marshal(decoded)
		if err != nil {
			return nil, fmt.Errorf("marshal YAML: %w", err)
		}

		return content, nil
	default:
		return nil, fmt.Errorf("unsupported agent file extension %q", extension)
	}
}

func stageImportRoot(stageRoot string, mode registry.AgentLoadOptions) (string, error) {
	if mode.ExclusiveDir != "" {
		return filepath.Join(stageRoot, "exclusive"), nil
	}

	projectRoot := filepath.Join(stageRoot, "project")

	return filepath.Join(projectRoot, defaultProjectAgentDir), nil
}

func copyAgentTree(srcRoot, dstRoot string) error {
	info, err := os.Stat(srcRoot)
	if os.IsNotExist(err) {
		return nil
	}

	if err != nil {
		return fmt.Errorf("stat import destination root %q: %w", srcRoot, err)
	}

	if !info.IsDir() {
		return fmt.Errorf("import destination root %q is not a directory", srcRoot)
	}

	return filepath.WalkDir(srcRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			if entry != nil && entry.IsDir() {
				return filepath.SkipDir
			}

			return fmt.Errorf("visit existing agent path %q: %w", path, walkErr)
		}

		relative, err := filepath.Rel(srcRoot, path)
		if err != nil {
			return fmt.Errorf("resolve existing agent path %q relative to %q: %w", path, srcRoot, err)
		}

		if relative == "." {
			return os.MkdirAll(dstRoot, setupDirMode)
		}

		target := filepath.Join(dstRoot, relative)
		if entry.IsDir() {
			return os.MkdirAll(target, setupDirMode)
		}

		if entry.Type()&os.ModeSymlink != 0 || !entry.Type().IsRegular() {
			return nil
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read existing agent file %q: %w", path, err)
		}

		if err := os.MkdirAll(filepath.Dir(target), setupDirMode); err != nil {
			return fmt.Errorf("create staged import directory: %w", err)
		}

		if err := os.WriteFile(target, content, setupFileMode); err != nil {
			return fmt.Errorf("write staged agent file %q: %w", target, err)
		}

		return nil
	})
}

func applyImportedAgentFiles(root, importRoot string, files []renderedImportedAgentFile) error {
	for _, file := range files {
		destination := filepath.Join(root, file.relativePath)
		if err := os.MkdirAll(filepath.Dir(destination), setupDirMode); err != nil {
			return fmt.Errorf("create import directory for %q: %w", filepath.Join(importRoot, file.relativePath), err)
		}

		if err := os.WriteFile(destination, file.content, setupFileMode); err != nil {
			return fmt.Errorf("write imported agent %q: %w", filepath.Join(importRoot, file.relativePath), err)
		}
	}

	return nil
}

func validateImportedRegistry(mode registry.AgentLoadOptions, stagedAgentsRoot string) error {
	options := mode
	if options.ExclusiveDir != "" {
		options.ExclusiveDir = stagedAgentsRoot
	} else {
		options.ProjectDir = stagedAgentsRoot
	}

	if _, err := registry.LoadAgents(options); err != nil {
		return fmt.Errorf("validate imported agents: %w", err)
	}

	return nil
}

func reportAgentImportResult(output io.Writer, result agentImportResult) error {
	if len(result.created) > 0 {
		if _, err := fmt.Fprintln(output, "Created imported agents:"); err != nil {
			return err
		}

		for _, path := range result.created {
			if _, err := fmt.Fprintf(output, "  %s\n", path); err != nil {
				return err
			}
		}
	}

	if len(result.overwritten) > 0 {
		if _, err := fmt.Fprintln(output, "Overwritten imported agents:"); err != nil {
			return err
		}

		for _, path := range result.overwritten {
			if _, err := fmt.Fprintf(output, "  %s\n", path); err != nil {
				return err
			}
		}
	}

	if len(result.unchanged) > 0 {
		if _, err := fmt.Fprintln(output, "Existing imported agents left unchanged:"); err != nil {
			return err
		}

		for _, path := range result.unchanged {
			if _, err := fmt.Fprintf(output, "  %s\n", path); err != nil {
				return err
			}
		}
	}

	if len(result.created) == 0 && len(result.overwritten) == 0 && len(result.unchanged) == 0 {
		_, err := fmt.Fprintln(output, "No imported agents were selected.")

		return err
	}

	return nil
}

func cleanImportRelativeDirectory(value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return defaultImportRemotePath, nil
	}

	clean := filepath.Clean(filepath.FromSlash(trimmed))
	if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes the repository root")
	}

	return clean, nil
}

func cleanImportPrefix(value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", nil
	}

	raw := filepath.ToSlash(trimmed)
	for _, segment := range strings.Split(raw, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return "", fmt.Errorf("prefix contains an empty or dot segment")
		}
	}

	clean := filepath.ToSlash(filepath.Clean(filepath.FromSlash(trimmed)))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || strings.HasPrefix(clean, "/") {
		return "", fmt.Errorf("prefix must be a relative namespace")
	}

	return clean, nil
}
