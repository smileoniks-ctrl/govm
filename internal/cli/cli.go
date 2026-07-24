package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/smileoniks-ctrl/govm/internal/install"
	"github.com/smileoniks-ctrl/govm/internal/paths"
	"github.com/smileoniks-ctrl/govm/internal/utils"
)

// InstallVersion resolves and installs a Go version from go.dev.
//
// The user-facing flow and messages mirror the previous implementation:
// it prints the lookup/install prompts, then delegates the long-running
// transactional install to a private, testable adapter driven by
// install.Service. No tea.Cmd/tea.Msg is produced or inspected here.
func InstallVersion(version string) {
	fmt.Printf("🔍 Looking for Go version matching %s...\n", version)
	matchedVersion, err := findMatchingVersion(version)
	if err != nil {
		fmt.Printf("❌ %s\n", err)
		return
	}
	fmt.Printf("📥 Installing Go %s...\n", matchedVersion.Version)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	svc := install.NewService()
	newInstallAdapter(matchedVersion, svc.Install, os.Stdout).run(ctx)
}

// installFunc mirrors install.(*Service).Install. It is parameterised so
// the CLI adapter can be exercised in tests without performing real
// network or disk work.
type installFunc func(ctx context.Context, req install.Request) (install.Result, error)

// installOutcome carries the service result and error together through a
// single buffered channel, so the adapter never needs separate done/err
// channels.
type installOutcome struct {
	result install.Result
	err    error
}

// installAdapter drives the animated spinner while a transactional
// installFunc runs. It is package-private and fully deterministic when
// constructed with a controllable tick channel and an injected install
// function.
type installAdapter struct {
	version  string
	request  install.Request
	install  installFunc
	out      io.Writer
	tick     <-chan time.Time // nil -> real ticker at spinRate
	spinRate time.Duration
}

// buildInstallRequest maps the full utils.GoVersion metadata (including
// integrity checksum and archive size) onto an install.Request.
func buildInstallRequest(v utils.GoVersion) install.Request {
	return install.Request{
		Version:  v.Version,
		Filename: v.Filename,
		URL:      v.URL,
		SHA256:   v.SHA256,
		Size:     v.Size,
	}
}

// newInstallAdapter wires the adapter for the public CLI path: output to
// stdout and a 100ms spinner cadence.
func newInstallAdapter(v utils.GoVersion, fn installFunc, out io.Writer) *installAdapter {
	return &installAdapter{
		version:  v.Version,
		request:  buildInstallRequest(v),
		install:  fn,
		out:      out,
		spinRate: 100 * time.Millisecond,
	}
}

// installSpinChars is the braille spinner animation shared by the CLI
// and TUI install flows.
var installSpinChars = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// run animates the spinner until the install func completes, then prints
// the success / warning / failure output to a.out.
//
// The install func runs in its own goroutine carrying ctx, so the service
// observes cancellation. On ctx cancellation the adapter does NOT abandon
// the worker: it disables the done case and keeps waiting for the service
// result (whose error carries the cancellation), which avoids stranding
// the goroutine and a resulting leak.
func (a *installAdapter) run(ctx context.Context) {
	if a.install == nil {
		a.finish(installOutcome{err: errors.New("no installer configured")})
		return
	}

	tick := a.tick
	if tick == nil {
		ticker := time.NewTicker(a.spinRate)
		defer ticker.Stop()
		tick = ticker.C
	}

	outcomeCh := make(chan installOutcome, 1)
	go func() {
		res, err := a.install(ctx, a.request)
		outcomeCh <- installOutcome{result: res, err: err}
	}()

	done := ctx.Done()
	idx := 0
	for {
		select {
		case o := <-outcomeCh:
			a.finish(o)
			return
		case <-tick:
			fmt.Fprintf(a.out, "\r%s Installing Go %s...", installSpinChars[idx], a.version)
			idx = (idx + 1) % len(installSpinChars)
		case <-done:
			// Deadline/cancellation reached. Do not return and strand the
			// worker: nil out done so this case stops firing, and keep
			// waiting for the service result, which reports the
			// cancellation as a phase-aware error.
			done = nil
		}
	}
}

// finish renders the terminal output for a completed install. The error
// path passes the typed install.Error through unchanged, so its
// phase-aware message and RecoveryPath remain visible.
func (a *installAdapter) finish(o installOutcome) {
	if o.err != nil {
		fmt.Fprintf(a.out, "\r❌ Installation failed: %v\n", o.err)
		return
	}
	fmt.Fprintf(a.out, "\r✅ Successfully installed Go %s\n", a.version)
	fmt.Fprintf(a.out, "👉 To activate this version, run: govm use %s\n", a.version)
	for _, w := range o.result.Warnings {
		fmt.Fprintf(a.out, "⚠️  %s\n", w)
	}
}
func UseVersion(version string) {
	fmt.Printf("🔍 Looking for installed Go version matching %s...\n", version)
	matchedVersion, err := findInstalledVersion(version)
	if err != nil {
		fmt.Printf("❌ %s\n", err)
		return
	}
	fmt.Printf("🔄 Switching to Go %s...\n", matchedVersion.Version)
	msg := utils.SwitchVersion(matchedVersion)()
	switch msg := msg.(type) {
	case utils.ErrMsg:
		fmt.Printf("❌ Failed to switch version: %v\n", msg)
	case utils.SwitchCompletedMsg:
		fmt.Printf("✅ Switched to Go %s\n", matchedVersion.Version)
		if !utils.IsShimInPath() {
			fmt.Println("\n⚠️  GoVM is not in your PATH")
			fmt.Println(utils.GetShimPathInstructions())
		} else {
			fmt.Println("🚀 Run 'go version' in a new terminal to verify")
		}
	}
}
func ListVersions() {
	fmt.Println("📋 Installed Go Versions:")
	resolver := paths.New()
	activeVersionFile, err := resolver.ActiveVersionFile()
	if err != nil {
		fmt.Printf("❌ Error resolving active version path: %v\n", err)
		return
	}
	activeVersion := ""
	if versionBytes, err := os.ReadFile(activeVersionFile); err == nil {
		activeVersion = string(versionBytes)
	}
	goVersionsDir, err := resolver.VersionsDir()
	if err != nil {
		fmt.Printf("❌ Error resolving versions directory: %v\n", err)
		return
	}
	if _, err := os.Stat(goVersionsDir); os.IsNotExist(err) {
		fmt.Println("  No versions installed yet")
		return
	}
	entries, err := os.ReadDir(goVersionsDir)
	if err != nil {
		fmt.Printf("❌ Error reading versions directory: %v\n", err)
		return
	}
	if len(entries) == 0 {
		fmt.Println("  No versions installed yet")
		return
	}
	for _, entry := range entries {
		if entry.IsDir() && strings.HasPrefix(entry.Name(), "go") {
			version := strings.TrimPrefix(entry.Name(), "go")
			if version == activeVersion {
				fmt.Printf("  %s %s\n", version, "✓ (active)")
			} else {
				fmt.Printf("  %s\n", version)
			}
		}
	}
	fmt.Println("\nTo install a new version: govm install <version>")
	fmt.Println("To switch versions: govm use <version>")
}

// findMatchingVersion looks up a Go version available on go.dev.
// It first checks for an exact match, then falls back to the highest
// version that starts with query (with or without a separating dot).
func findMatchingVersion(version string) (utils.GoVersion, error) {
	msg := utils.FetchGoVersions()
	versions, ok := msg.(utils.VersionsMsg)
	if !ok {
		if errMsg, isErr := msg.(utils.ErrMsg); isErr {
			return utils.GoVersion{}, fmt.Errorf("failed to fetch versions: %v", errMsg)
		}
		return utils.GoVersion{}, fmt.Errorf("failed to fetch versions")
	}

	query := utils.NormalizeGoVersionQuery(version)
	versionStrings := make([]string, len(versions))
	for i, v := range versions {
		versionStrings[i] = v.Version
	}
	matched, ok := utils.FindLatestGoVersion(versionStrings, query)
	if !ok {
		return utils.GoVersion{}, fmt.Errorf("no version matching '%s' found", version)
	}
	for _, v := range versions {
		if v.Version == matched {
			return v, nil
		}
	}
	return utils.GoVersion{}, fmt.Errorf("no version matching '%s' found", version)
}

// findInstalledVersion mirrors findMatchingVersion but reads the
// installed govm versions directly from disk so the CLI works
// without contacting go.dev. It shares the same disk view as the
// TUI via utils.ScanInstalledVersions (W-fix for candidate 9).
func findInstalledVersion(version string) (utils.GoVersion, error) {
	resolver := paths.New()
	goVersionsDir, err := resolver.VersionsDir()
	if err != nil {
		return utils.GoVersion{}, fmt.Errorf("failed to get home directory: %v", err)
	}

	installed, err := utils.ScanInstalledVersions(goVersionsDir)
	if err != nil {
		return utils.GoVersion{}, fmt.Errorf("failed to read versions directory: %v", err)
	}

	query := utils.NormalizeGoVersionQuery(version)
	versions := make([]string, 0, len(installed))
	for v := range installed {
		versions = append(versions, v)
	}

	matched, ok := utils.FindLatestGoVersion(versions, query)
	if !ok {
		return utils.GoVersion{}, fmt.Errorf("no installed version matching '%s' found", version)
	}
	return utils.GoVersion{
		Version:   matched,
		Path:      installed[matched],
		Installed: true,
	}, nil
}

func DeleteVersion(version string) {
	fmt.Printf("🔍 Looking for installed Go version matching %s...\n", version)
	matchedVersion, err := findInstalledVersion(version)
	if err != nil {
		fmt.Printf("❌ %s\n", err)
		return
	}

	resolver := paths.New()
	activeVersionFile, err := resolver.ActiveVersionFile()
	if err != nil {
		fmt.Printf("❌ Failed to get home directory: %v\n", err)
		return
	}

	activeVersion := ""
	if versionBytes, err := os.ReadFile(activeVersionFile); err == nil {
		activeVersion = string(versionBytes)
	}

	if matchedVersion.Version == activeVersion {
		fmt.Printf("❌ Cannot delete active version. Switch to another version first using 'govm use'.\n")
		return
	}

	fmt.Printf("⚠️  Are you sure you want to delete Go %s? (y/N): ", matchedVersion.Version)
	var response string
	fmt.Scanln(&response)

	if strings.ToLower(response) != "y" {
		fmt.Println("🛑 Operation canceled.")
		return
	}

	fmt.Printf("🗑️  Deleting Go %s...\n", matchedVersion.Version)

	msg := utils.DeleteVersion(matchedVersion)()
	switch msg := msg.(type) {
	case utils.ErrMsg:
		fmt.Printf("❌ Failed to delete version: %v\n", msg)
	case utils.DeleteCompleteMsg:
		fmt.Printf("✅ Successfully deleted Go %s\n", matchedVersion.Version)
	}
}

// DepsCommand routes `govm deps <subcommand>`.
func DepsCommand(args ...string) {
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Printf("❌ Error getting working directory: %v\n", err)
		return
	}
	subcommand := "help"
	if len(args) > 0 {
		subcommand = args[0]
	}
	switch subcommand {
	case "list":
		DepsList(cwd)
	case "check":
		DepsCheck(cwd)
	case "update":
		DepsUpdate(cwd)
	case "backups":
		DepsBackups(cwd)
	case "restore":
		name := ""
		if len(args) > 1 {
			name = args[1]
		}
		DepsRestore(cwd, name)
	case "help", "-h", "--help":
		fmt.Println("Usage:")
		fmt.Println("  govm deps list              List current module dependencies")
		fmt.Println("  govm deps check             Check for available dependency updates")
		fmt.Println("  govm deps update            Update direct dependencies (interactive)")
		fmt.Println("  govm deps backups           List dependency backups")
		fmt.Println("  govm deps restore <file>    Restore dependency backup")
	default:
		fmt.Printf("Unknown deps subcommand: %s\n", subcommand)
		fmt.Println("Run 'govm deps help' for usage.")
	}
}

// DepsList prints the current dependencies of moduleDir.
func DepsList(moduleDir string) {
	svc := NewDepsService(moduleDir, os.Stdout, os.Stdin)
	if err := svc.RunList(); err != nil {
		fmt.Printf("❌ %s\n", err)
	}
}

// DepsCheck prints the dependencies along with any available updates.
func DepsCheck(moduleDir string) {
	svc := NewDepsService(moduleDir, os.Stdout, os.Stdin)
	if err := svc.RunCheck(); err != nil {
		fmt.Printf("❌ %s\n", err)
	}
}

// DepsUpdate runs the interactive update workflow.
func DepsUpdate(moduleDir string) {
	svc := NewDepsService(moduleDir, os.Stdout, os.Stdin)
	if err := svc.RunUpdate(); err != nil {
		fmt.Printf("❌ %s\n", err)
	}
}

// DepsBackups lists saved dependency backups.
func DepsBackups(moduleDir string) {
	svc := NewDepsService(moduleDir, os.Stdout, os.Stdin)
	if err := svc.RunBackups(); err != nil {
		fmt.Printf("❌ %s\n", err)
	}
}

// DepsRestore restores a saved dependency backup.
func DepsRestore(moduleDir, name string) {
	svc := NewDepsService(moduleDir, os.Stdout, os.Stdin)
	if err := svc.RunRestore(name); err != nil {
		fmt.Printf("❌ %s\n", err)
	}
}
