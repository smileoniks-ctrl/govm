package cli

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/smileoniks-ctrl/govm/internal/config"
	"github.com/smileoniks-ctrl/govm/internal/utils"
)

// DepsService encapsulates the CLI dependency workflow and lets tests
// substitute individual operations.
type DepsService struct {
	ModuleDir string
	Stdout    io.Writer
	Stdin     io.Reader

	// Confirm asks the user a yes/no question. The default answer
	// (used on empty input or EOF) is defaultYes. The default
	// implementation writes the question to Stdout and reads a
	// line from Stdin.
	Confirm func(question string, defaultYes bool) (bool, error)

	// ListDeps returns the current dependencies of moduleDir.
	ListDeps func(moduleDir string) ([]utils.ModuleDependency, error)

	// CheckDeps returns the dependencies of moduleDir together with
	// available update information.
	CheckDeps func(moduleDir string) ([]utils.ModuleDependency, error)

	// Update runs go get + go mod tidy and returns the resulting message.
	Update func(moduleDir string, deps []utils.ModuleDependency) (utils.DependenciesUpdatedMsg, error)

	// RunChecks runs go test ./... and go vet ./....
	RunChecks func(moduleDir string) (utils.DependencyCheckResultMsg, error)

	// Rollback restores the snapshot and runs go mod tidy.
	Rollback func(moduleDir string, snap *utils.DependencySnapshot) (utils.DependenciesRolledBackMsg, error)

	// ListBackups returns saved dependency backups for moduleDir.
	ListBackups func(moduleDir string) ([]utils.DependencyBackupInfo, error)

	// RestoreBackup restores a saved dependency backup by filename.
	RestoreBackup func(moduleDir, name string) (utils.DependenciesRestoredMsg, error)
}

// NewDepsService builds a service that defaults each operation to the
// corresponding helper in the utils package.
func NewDepsService(moduleDir string, stdout io.Writer, stdin io.Reader) *DepsService {
	if stdout == nil {
		stdout = os.Stdout
	}
	if stdin == nil {
		stdin = os.Stdin
	}
	settings, err := config.Load("")
	if err != nil {
		settings = config.DefaultSettings()
	}
	backupLimit := settings.DepsBackupLimit
	return &DepsService{
		ModuleDir: moduleDir,
		Stdout:    stdout,
		Stdin:     stdin,
		Confirm:   defaultConfirm(stdin, stdout),
		ListDeps:  defaultListDeps,
		CheckDeps: defaultCheckDeps,
		Update: func(moduleDir string, deps []utils.ModuleDependency) (utils.DependenciesUpdatedMsg, error) {
			return defaultUpdate(moduleDir, deps, backupLimit)
		},
		RunChecks:   defaultRunChecks,
		Rollback:    defaultRollback,
		ListBackups: defaultListBackups,
		RestoreBackup: func(moduleDir, name string) (utils.DependenciesRestoredMsg, error) {
			return defaultRestoreBackup(moduleDir, name, backupLimit)
		},
	}
}

func defaultListDeps(moduleDir string) ([]utils.ModuleDependency, error) {
	msg := utils.ListModuleDependencies(moduleDir)()
	deps, ok := msg.(utils.DependenciesMsg)
	if !ok {
		if errMsg, ok := msg.(utils.DependencyErrMsg); ok {
			return nil, errMsg.Err
		}
		return nil, fmt.Errorf("unexpected list result: %T", msg)
	}
	return []utils.ModuleDependency(deps), nil
}

func defaultCheckDeps(moduleDir string) ([]utils.ModuleDependency, error) {
	msg := utils.CheckModuleDependencyUpdates(moduleDir)()
	deps, ok := msg.(utils.DependenciesMsg)
	if !ok {
		if errMsg, ok := msg.(utils.DependencyErrMsg); ok {
			return nil, errMsg.Err
		}
		return nil, fmt.Errorf("unexpected check result: %T", msg)
	}
	return []utils.ModuleDependency(deps), nil
}

func defaultUpdate(moduleDir string, deps []utils.ModuleDependency, backupLimit int) (utils.DependenciesUpdatedMsg, error) {
	msg := utils.UpdateModuleDependencies(moduleDir, deps, backupLimit)()
	if updated, ok := msg.(utils.DependenciesUpdatedMsg); ok {
		return updated, nil
	}
	if errMsg, ok := msg.(utils.DependencyErrMsg); ok {
		return utils.DependenciesUpdatedMsg{}, errMsg.Err
	}
	return utils.DependenciesUpdatedMsg{}, fmt.Errorf("unexpected update result: %T", msg)
}

func defaultRunChecks(moduleDir string) (utils.DependencyCheckResultMsg, error) {
	msg := utils.RunModuleDependencyChecks(moduleDir)()
	if res, ok := msg.(utils.DependencyCheckResultMsg); ok {
		return res, nil
	}
	if errMsg, ok := msg.(utils.DependencyErrMsg); ok {
		return utils.DependencyCheckResultMsg{}, errMsg.Err
	}
	return utils.DependencyCheckResultMsg{}, fmt.Errorf("unexpected check result: %T", msg)
}

func defaultRollback(moduleDir string, snap *utils.DependencySnapshot) (utils.DependenciesRolledBackMsg, error) {
	msg := utils.RollbackModuleDependencies(moduleDir, snap)()
	if rolled, ok := msg.(utils.DependenciesRolledBackMsg); ok {
		return rolled, nil
	}
	if errMsg, ok := msg.(utils.DependencyErrMsg); ok {
		return utils.DependenciesRolledBackMsg{}, errMsg.Err
	}
	return utils.DependenciesRolledBackMsg{}, fmt.Errorf("unexpected rollback result: %T", msg)
}

func defaultListBackups(moduleDir string) ([]utils.DependencyBackupInfo, error) {
	msg := utils.ListDependencyBackupsCmd(moduleDir)()
	backups, ok := msg.(utils.DependencyBackupsMsg)
	if ok {
		return []utils.DependencyBackupInfo(backups), nil
	}
	if errMsg, ok := msg.(utils.DependencyErrMsg); ok {
		return nil, errMsg.Err
	}
	return nil, fmt.Errorf("unexpected backup list result: %T", msg)
}

func defaultRestoreBackup(moduleDir, name string, backupLimit int) (utils.DependenciesRestoredMsg, error) {
	msg := utils.RestoreDependencyBackup(moduleDir, name, backupLimit)()
	if restored, ok := msg.(utils.DependenciesRestoredMsg); ok {
		return restored, nil
	}
	if errMsg, ok := msg.(utils.DependencyErrMsg); ok {
		return utils.DependenciesRestoredMsg{}, errMsg.Err
	}
	return utils.DependenciesRestoredMsg{}, fmt.Errorf("unexpected restore result: %T", msg)
}

// defaultConfirm returns a Confirm implementation that prompts on
// stdout and reads a single line from stdin.
func defaultConfirm(stdin io.Reader, stdout io.Writer) func(string, bool) (bool, error) {
	scanner := bufio.NewScanner(stdin)
	return func(question string, defaultYes bool) (bool, error) {
		prompt := fmt.Sprintf("%s [%s]: ", question, yesNoLabel(defaultYes))
		for {
			if _, err := io.WriteString(stdout, prompt); err != nil {
				return false, err
			}
			if !scanner.Scan() {
				if err := scanner.Err(); err != nil && !errors.Is(err, io.EOF) {
					return false, err
				}
				return defaultYes, nil
			}
			answer := strings.TrimSpace(scanner.Text())
			if answer == "" {
				return defaultYes, nil
			}
			switch strings.ToLower(answer) {
			case "y", "yes":
				return true, nil
			case "n", "no":
				return false, nil
			default:
				if _, err := io.WriteString(stdout, "Please answer y or n.\n"); err != nil {
					return false, err
				}
			}
		}
	}
}

func yesNoLabel(defaultYes bool) string {
	if defaultYes {
		return "Y/n"
	}
	return "y/N"
}

// RunList prints the current module dependencies to Stdout.
func (s *DepsService) RunList() error {
	deps, err := s.ListDeps(s.ModuleDir)
	if err != nil {
		return fmt.Errorf("failed to read dependencies: %w", err)
	}
	direct, indirect := 0, 0
	for _, d := range deps {
		if d.Indirect {
			indirect++
		} else {
			direct++
		}
	}
	fmt.Fprintf(s.Stdout, "🔍 Reading module dependencies in %s...\n\n", s.ModuleDir)
	if len(deps) == 0 {
		fmt.Fprintln(s.Stdout, "  (no dependencies)")
	} else {
		for _, line := range formatDepRows(deps) {
			fmt.Fprintf(s.Stdout, "  %s\n", line)
		}
	}
	fmt.Fprintf(s.Stdout, "\n✅ %d direct, %d indirect dependencies.\n", direct, indirect)
	return nil
}

func formatDepRows(deps []utils.ModuleDependency) []string {
	rows := make([]string, 0, len(deps))
	for _, d := range deps {
		kind := "direct"
		if d.Indirect {
			kind = "indirect"
		}
		rows = append(rows, fmt.Sprintf("%s\t%s\t%s", d.Path, d.Version, kind))
	}
	return rows
}

// RunCheck prints the dependencies and marks available updates.
func (s *DepsService) RunCheck() error {
	deps, err := s.CheckDeps(s.ModuleDir)
	if err != nil {
		return fmt.Errorf("failed to check dependencies: %w", err)
	}
	updates := countDirectUpdates(deps)
	fmt.Fprintf(s.Stdout, "🔍 Checking available updates in %s...\n\n", s.ModuleDir)
	if len(deps) == 0 {
		fmt.Fprintln(s.Stdout, "  (no dependencies)")
	} else {
		for _, line := range formatCheckRows(deps) {
			fmt.Fprintf(s.Stdout, "  %s\n", line)
		}
	}
	if updates == 0 {
		fmt.Fprintln(s.Stdout, "\n✅ 0 direct updates available.")
	} else {
		fmt.Fprintf(s.Stdout, "\n📦 %d direct update(s) available.\n", updates)
	}
	return nil
}

// RunBackups prints saved dependency backups for the current module.
func (s *DepsService) RunBackups() error {
	backups, err := s.ListBackups(s.ModuleDir)
	if err != nil {
		return fmt.Errorf("failed to list dependency backups: %w", err)
	}
	fmt.Fprintf(s.Stdout, "📦 Dependency backups in %s:\n\n", s.ModuleDir)
	if len(backups) == 0 {
		fmt.Fprintln(s.Stdout, "  (no backups)")
		return nil
	}
	for _, b := range backups {
		fmt.Fprintf(
			s.Stdout,
			"  %s\t%s\t%s\t%d update(s)\n",
			b.Name,
			b.ModulePath,
			b.Kind,
			b.Updated,
		)
	}
	return nil
}

// RunRestore restores one saved dependency backup by filename.
func (s *DepsService) RunRestore(name string) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("backup filename is required")
	}
	ok, err := s.Confirm(fmt.Sprintf("Restore dependency backup %s?", name), true)
	if err != nil {
		return err
	}
	if !ok {
		fmt.Fprintln(s.Stdout, "🛑 Restore canceled.")
		return nil
	}
	restored, err := s.RestoreBackup(s.ModuleDir, name)
	if err != nil {
		return fmt.Errorf("restore failed: %w", err)
	}
	fmt.Fprintf(
		s.Stdout,
		"✅ Restored dependencies from %s (%s).\n",
		restored.BackupName,
		restored.BackupCreated.Format("2006-01-02 15:04:05"),
	)
	return nil
}

func countDirectUpdates(deps []utils.ModuleDependency) int {
	return len(utils.UpdatableDirectDependencies(deps))
}

func formatCheckRows(deps []utils.ModuleDependency) []string {
	rows := make([]string, 0, len(deps))
	for _, d := range deps {
		status := "current"
		version := d.Version
		switch {
		case d.Error != "":
			status = "error: " + d.Error
		case d.Deprecated != "" && d.Latest != "" && d.Latest != d.Version:
			status = "update available (deprecated)"
		case d.Latest != "" && d.Latest != d.Version:
			status = "update available"
			version = fmt.Sprintf("%s → %s", d.Version, d.Latest)
		case d.Deprecated != "":
			status = "deprecated"
		}
		rows = append(rows, fmt.Sprintf("%s\t%s\t%s", d.Path, version, status))
	}
	return rows
}

// RunUpdate runs the full scenario: check, confirm, update, optional
// checks, optional rollback.
func (s *DepsService) RunUpdate() error {
	fmt.Fprintf(s.Stdout, "🔍 Checking available updates in %s...\n", s.ModuleDir)
	deps, err := s.CheckDeps(s.ModuleDir)
	if err != nil {
		return fmt.Errorf("failed to check dependencies: %w", err)
	}
	updatable := utils.UpdatableDirectDependencies(deps)
	if len(updatable) == 0 {
		fmt.Fprintln(s.Stdout, "ℹ️  No direct dependency updates available.")
		return nil
	}

	fmt.Fprintf(s.Stdout, "\n⚠️  %d direct %s will be updated:\n",
		len(updatable), pluralize(len(updatable), "dependency", "dependencies"))
	for _, line := range formatUpdateEntries(buildUpdateEntries(updatable)) {
		fmt.Fprintf(s.Stdout, "  - %s\n", line)
	}
	ok, err := s.Confirm("\nApply these updates?", true)
	if err != nil {
		return err
	}
	if !ok {
		fmt.Fprintln(s.Stdout, "🛑 Update canceled.")
		return nil
	}

	updated, err := s.Update(s.ModuleDir, deps)
	if err != nil {
		return fmt.Errorf("update failed: %w", err)
	}
	fmt.Fprintf(s.Stdout, "✅ Updated %d direct %s.\n",
		updated.Updated, pluralize(updated.Updated, "dependency", "dependencies"))
	if updated.Snapshot == nil {
		return fmt.Errorf("update completed without a rollback snapshot")
	}

	ok, err = s.Confirm("\n🧪 Run checks (go test ./... and go vet ./...)?", true)
	if err != nil {
		return err
	}
	if !ok {
		fmt.Fprintln(s.Stdout, "ℹ️  Update complete. Checks skipped.")
		return nil
	}

	result, err := s.RunChecks(s.ModuleDir)
	if err != nil {
		return fmt.Errorf("checks failed to run: %w", err)
	}
	if result.OK {
		fmt.Fprintln(s.Stdout, "✅ Checks passed.")
		return nil
	}

	fmt.Fprintf(s.Stdout, "\n❌ Checks failed: %s\n", result.Command)
	if result.Output != "" {
		fmt.Fprintln(s.Stdout, result.Output)
	}
	ok, err = s.Confirm("\nRoll back to pre-update state?", true)
	if err != nil {
		return err
	}
	if !ok {
		fmt.Fprintln(s.Stdout, "⚠️  Update kept. Failed checks were not rolled back.")
		return nil
	}
	if _, err := s.Rollback(s.ModuleDir, updated.Snapshot); err != nil {
		return fmt.Errorf("rollback failed: %w", err)
	}
	fmt.Fprintln(s.Stdout, "✅ Rolled back to pre-update state.")
	return nil
}

func buildUpdateEntries(deps []utils.ModuleDependency) []utils.DependencyUpdateEntry {
	entries := make([]utils.DependencyUpdateEntry, 0, len(deps))
	for _, d := range deps {
		entries = append(entries, utils.DependencyUpdateEntry{
			Path:       d.Path,
			OldVersion: d.Version,
			NewVersion: d.Latest,
		})
	}
	return entries
}

func formatUpdateEntries(entries []utils.DependencyUpdateEntry) []string {
	rows := make([]string, 0, len(entries))
	for _, e := range entries {
		rows = append(rows, fmt.Sprintf("%s  %s → %s", e.Path, e.OldVersion, e.NewVersion))
	}
	return rows
}

func pluralize(n int, singular, plural string) string {
	if n == 1 {
		return singular
	}
	return plural
}
