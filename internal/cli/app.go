package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/smileoniks-ctrl/govm/internal/lifecycle"
	"github.com/smileoniks-ctrl/govm/internal/paths"
	"github.com/smileoniks-ctrl/govm/internal/prune"
	"github.com/smileoniks-ctrl/govm/internal/services"
	"github.com/smileoniks-ctrl/govm/internal/utils"
)

type activateFunc func(context.Context, string) (lifecycle.ActivationResult, error)
type deleteFunc func(context.Context, string) (lifecycle.DeletionResult, error)
type prunePreviewFunc func(context.Context) (prune.Result, error)
type pruneFunc func(context.Context) (prune.Result, error)

type pathResolver interface {
	ActiveVersionFile() (string, error)
	VersionsDir() (string, error)
}

// InstallVersion resolves and installs a Go version from the configured
// distribution source.
func (a *App) InstallVersion(version string) {
	fmt.Fprintf(a.out, "🔍 Looking for Go version matching %s...\n", version)
	matchedVersion, err := findMatchingVersion(a.operations.Runtime, version)
	if err != nil {
		fmt.Fprintf(a.out, "❌ %s\n", err)
		return
	}
	fmt.Fprintf(a.out, "📥 Installing Go %s...\n", matchedVersion.Version)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	newInstallAdapter(matchedVersion, a.operations.Install, a.out).run(ctx)
}

// Operations contains the narrow core seams used by App.
type Operations struct {
	Runtime      *services.Runtime
	Install      installFunc
	Activate     activateFunc
	Delete       deleteFunc
	PreviewPrune prunePreviewFunc
	Prune        pruneFunc
	Resolver     pathResolver
	ShimInPath   func() bool
}

// App maps CLI commands and process I/O onto core operations.
type App struct {
	operations Operations
	in         io.Reader
	out        io.Writer
	errOut     io.Writer
}

// NewApp constructs a testable CLI adapter.
func NewApp(operations Operations, in io.Reader, out, errOut io.Writer) *App {
	if in == nil {
		in = strings.NewReader("")
	}
	if out == nil {
		out = io.Discard
	}
	if errOut == nil {
		errOut = io.Discard
	}
	if operations.Resolver == nil {
		operations.Resolver = paths.New()
	}
	if operations.ShimInPath == nil {
		operations.ShimInPath = utils.IsShimInPath
	}
	return &App{operations: operations, in: in, out: out, errOut: errOut}
}

// UseVersion resolves and activates an installed Go version.
func (a *App) UseVersion(version string) {
	fmt.Fprintf(a.out, "🔍 Looking for installed Go version matching %s...\n", version)
	matchedVersion, err := findInstalledVersion(version)
	if err != nil {
		fmt.Fprintf(a.out, "❌ %s\n", err)
		return
	}
	fmt.Fprintf(a.out, "🔄 Switching to Go %s...\n", matchedVersion.Version)
	if a.operations.Activate == nil {
		fmt.Fprintln(a.out, "❌ Failed to switch version: no lifecycle activator configured")
		return
	}
	result, err := a.operations.Activate(context.Background(), matchedVersion.Version)
	if err != nil {
		fmt.Fprintf(a.out, "❌ Failed to switch version: %v\n", err)
		return
	}
	fmt.Fprintf(a.out, "✅ Switched to Go %s\n", matchedVersion.Version)
	for _, warning := range result.Warnings {
		fmt.Fprintf(a.out, "⚠️  %s\n", warning)
	}
	if !a.operations.ShimInPath() {
		fmt.Fprintln(a.out, "\n⚠️  GoVM is not in your PATH")
		fmt.Fprintln(a.out, utils.GetShimPathInstructions())
		return
	}
	fmt.Fprintln(a.out, "🚀 Run 'go version' in a new terminal to verify")
}

// ListVersions prints installed Go versions.
func (a *App) ListVersions() {
	fmt.Fprintln(a.out, "📋 Installed Go Versions:")
	activeVersionFile, err := a.operations.Resolver.ActiveVersionFile()
	if err != nil {
		fmt.Fprintf(a.out, "❌ Error resolving active version path: %v\n", err)
		return
	}
	activeVersion := ""
	if versionBytes, err := os.ReadFile(activeVersionFile); err == nil {
		activeVersion = string(versionBytes)
	}
	goVersionsDir, err := a.operations.Resolver.VersionsDir()
	if err != nil {
		fmt.Fprintf(a.out, "❌ Error resolving versions directory: %v\n", err)
		return
	}
	entries, err := os.ReadDir(goVersionsDir)
	if os.IsNotExist(err) {
		fmt.Fprintln(a.out, "  No versions installed yet")
		return
	}
	if err != nil {
		fmt.Fprintf(a.out, "❌ Error reading versions directory: %v\n", err)
		return
	}
	if len(entries) == 0 {
		fmt.Fprintln(a.out, "  No versions installed yet")
		return
	}
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), "go") {
			continue
		}
		version := strings.TrimPrefix(entry.Name(), "go")
		if version == activeVersion {
			fmt.Fprintf(a.out, "  %s ✓ (active)\n", version)
		} else {
			fmt.Fprintf(a.out, "  %s\n", version)
		}
	}
	fmt.Fprintln(a.out, "\nTo install a new version: govm install <version>")
	fmt.Fprintln(a.out, "To switch versions: govm use <version>")
}

// DeleteVersion confirms and deletes an installed Go version.
func (a *App) DeleteVersion(version string) {
	fmt.Fprintf(a.out, "🔍 Looking for installed Go version matching %s...\n", version)
	matchedVersion, err := findInstalledVersion(version)
	if err != nil {
		fmt.Fprintf(a.out, "❌ %s\n", err)
		return
	}

	activeVersionFile, err := a.operations.Resolver.ActiveVersionFile()
	if err != nil {
		fmt.Fprintf(a.out, "❌ Failed to get home directory: %v\n", err)
		return
	}
	activeVersion := ""
	if versionBytes, err := os.ReadFile(activeVersionFile); err == nil {
		activeVersion = string(versionBytes)
	}
	if matchedVersion.Version == activeVersion {
		fmt.Fprintln(a.out, "❌ Cannot delete active version. Switch to another version first using 'govm use'.")
		return
	}

	fmt.Fprintf(a.out, "⚠️  Are you sure you want to delete Go %s? (y/N): ", matchedVersion.Version)
	var response string
	if _, err := fmt.Fscan(a.in, &response); err != nil && err != io.EOF {
		fmt.Fprintf(a.errOut, "Failed to read confirmation: %v\n", err)
	}
	if !strings.EqualFold(response, "y") {
		fmt.Fprintln(a.out, "🛑 Operation canceled.")
		return
	}

	fmt.Fprintf(a.out, "🗑️  Deleting Go %s...\n", matchedVersion.Version)
	if a.operations.Delete == nil {
		fmt.Fprintln(a.out, "❌ Failed to delete version: no lifecycle deleter configured")
		return
	}
	result, err := a.operations.Delete(context.Background(), matchedVersion.Version)
	if err != nil {
		fmt.Fprintf(a.out, "❌ Failed to delete version: %v\n", err)
		return
	}
	fmt.Fprintf(a.out, "✅ Successfully deleted Go %s\n", matchedVersion.Version)
	for _, warning := range result.Warnings {
		fmt.Fprintf(a.out, "⚠️  %s\n", warning)
	}
}

// PruneVersions previews and optionally removes inactive toolchains and
// govm-owned temporary downloads.
func (a *App) PruneVersions(args ...string) bool {
	yes, dryRun, err := parsePruneArgs(args)
	if err != nil {
		fmt.Fprintf(a.out, "Error: %v\n", err)
		return false
	}
	if a.operations.PreviewPrune == nil || a.operations.Prune == nil {
		fmt.Fprintln(a.out, "Error: prune service is not configured")
		return false
	}

	result, previewErr := a.operations.PreviewPrune(context.Background())
	printPrunePlan(a.out, result, dryRun)
	for _, warning := range result.Warnings {
		fmt.Fprintf(a.out, "Warning: %s\n", warning)
	}
	if previewErr != nil {
		fmt.Fprintf(a.out, "Error: cannot safely prune versions: %v\n", previewErr)
		if len(result.Candidates) == 0 {
			return false
		}
	}
	if len(result.Candidates) == 0 {
		fmt.Fprintln(a.out, "Nothing to prune.")
		return previewErr == nil
	}
	if dryRun {
		return previewErr == nil
	}
	if !yes {
		fmt.Fprint(a.out, "Are you sure? (y/N): ")
	}
	if !yes && !confirm(a.in) {
		fmt.Fprintln(a.out, "Operation canceled.")
		return true
	}

	fmt.Fprintln(a.out, "Pruning...")
	removed, pruneErr := a.operations.Prune(context.Background())
	for _, candidate := range removed.Removed {
		fmt.Fprintf(a.out, "Removed %s (%d bytes)\n", candidate.Path, candidate.Bytes)
	}
	for _, warning := range removed.Warnings {
		fmt.Fprintf(a.out, "Warning: %s\n", warning)
	}
	if pruneErr != nil {
		fmt.Fprintf(a.out, "Error: prune completed with warnings: %v\n", pruneErr)
		return false
	}
	fmt.Fprintf(a.out, "Freed %d bytes.\n", pruneRemovedBytes(removed))
	return true
}

func parsePruneArgs(args []string) (yes, dryRun bool, err error) {
	for _, arg := range args {
		switch arg {
		case "--yes", "-y":
			yes = true
		case "--dry-run":
			dryRun = true
		case "--help", "-h":
			return false, false, errors.New("usage: govm prune [--yes] [--dry-run]")
		default:
			return false, false, fmt.Errorf("unknown prune option %q", arg)
		}
	}
	return yes, dryRun, nil
}

func confirm(in io.Reader) bool {
	var response string
	if _, err := fmt.Fscan(in, &response); err != nil {
		return false
	}
	return strings.EqualFold(response, "y")
}

func printPrunePlan(out io.Writer, result prune.Result, dryRun bool) {
	action := "Would remove"
	if !dryRun {
		action = "Plan to remove"
	}
	fmt.Fprintf(out, "%s %d object(s), %d bytes:\n", action, len(result.Candidates), pruneResultBytes(result))
	for _, candidate := range result.Candidates {
		fmt.Fprintf(out, "  %s (%d bytes)\n", candidate.Path, candidate.Bytes)
	}
}

func pruneResultBytes(result prune.Result) int64 {
	var total int64
	for _, candidate := range result.Candidates {
		total += candidate.Bytes
	}
	return total
}

func pruneRemovedBytes(result prune.Result) int64 {
	var total int64
	for _, candidate := range result.Removed {
		total += candidate.Bytes
	}
	return total
}

// DepsCommand routes `govm deps <subcommand>`.
func (a *App) DepsCommand(args ...string) {
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(a.out, "❌ Error getting working directory: %v\n", err)
		return
	}
	subcommand := "help"
	if len(args) > 0 {
		subcommand = args[0]
	}
	service, err := NewDepsService(cwd, a.out, a.in)
	if err != nil {
		fmt.Fprintf(a.out, "❌ Error: %v\n", err)
		return
	}
	switch subcommand {
	case "list":
		err = service.RunList()
	case "check":
		err = service.RunCheck()
	case "update":
		err = service.RunUpdate()
	case "backups":
		err = service.RunBackups()
	case "restore":
		name := ""
		if len(args) > 1 {
			name = args[1]
		}
		err = service.RunRestore(name)
	case "help", "-h", "--help":
		fmt.Fprintln(a.out, "Usage:")
		fmt.Fprintln(a.out, "  govm deps list              List current module dependencies")
		fmt.Fprintln(a.out, "  govm deps check             Check for available dependency updates")
		fmt.Fprintln(a.out, "  govm deps update            Update direct dependencies (interactive)")
		fmt.Fprintln(a.out, "  govm deps backups           List dependency backups")
		fmt.Fprintln(a.out, "  govm deps restore <file>    Restore dependency backup")
		return
	default:
		fmt.Fprintf(a.out, "Unknown deps subcommand: %s\n", subcommand)
		fmt.Fprintln(a.out, "Run 'govm deps help' for usage.")
		return
	}
	if err != nil {
		fmt.Fprintf(a.out, "❌ %s\n", err)
	}
}
