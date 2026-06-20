package utils

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// ModuleDependency represents a single Go module dependency.
type ModuleDependency struct {
	Path       string
	Version    string
	Latest     string
	Indirect   bool
	Deprecated string
	Error      string
}

// DependenciesMsg carries the list of module dependencies.
type DependenciesMsg []ModuleDependency

// DependencyErrMsg carries a dependency-related error
// without affecting the main error state.
// Note: this is intentionally a plain struct (not an error) so that it
// does not satisfy the `error` interface and therefore does not collide
// with `utils.ErrMsg` in type switches.
type DependencyErrMsg struct {
	Err error
}

// DependenciesUpdatedMsg is sent after a successful update of direct
// dependencies, carrying the refreshed list and the number of modules
// that were upgraded.
type DependenciesUpdatedMsg struct {
	Updated      int
	Dependencies []ModuleDependency
}

// UpdatableDirectDependencies returns direct dependencies that have
// an available update.
func UpdatableDirectDependencies(deps []ModuleDependency) []ModuleDependency {
	var out []ModuleDependency
	for _, d := range deps {
		if d.Indirect {
			continue
		}
		if d.Error != "" {
			continue
		}
		if d.Latest == "" || d.Latest == d.Version {
			continue
		}
		out = append(out, d)
	}
	return out
}

// UpdateModuleDependencies runs `go get` for each updatable direct
// dependency in deps, then `go mod tidy`, and finally re-checks
// available updates.
func UpdateModuleDependencies(moduleDir string, deps []ModuleDependency) tea.Cmd {
	updatable := UpdatableDirectDependencies(deps)
	if len(updatable) == 0 {
		return func() tea.Msg {
			return DependencyErrMsg{Err: fmt.Errorf("no direct dependency updates available")}
		}
	}

	return func() tea.Msg {
		args := []string{"get"}
		for _, d := range updatable {
			args = append(args, fmt.Sprintf("%s@%s", d.Path, d.Latest))
		}

		getCmd := exec.Command("go", args...)
		getCmd.Dir = moduleDir
		if out, err := getCmd.CombinedOutput(); err != nil {
			return DependencyErrMsg{Err: fmt.Errorf("go get failed: %s: %w", strings.TrimSpace(string(out)), err)}
		}

		tidyCmd := exec.Command("go", "mod", "tidy")
		tidyCmd.Dir = moduleDir
		if out, err := tidyCmd.CombinedOutput(); err != nil {
			return DependencyErrMsg{Err: fmt.Errorf("go mod tidy failed: %s: %w", strings.TrimSpace(string(out)), err)}
		}

		// Refresh dependency list with available updates so the user
		// can see the new state in the table.
		fresh := loadDependencies(moduleDir, true)
		if errMsg, ok := fresh.(DependencyErrMsg); ok {
			return errMsg
		}
		depsMsg, ok := fresh.(DependenciesMsg)
		if !ok {
			return DependencyErrMsg{Err: fmt.Errorf("unexpected refresh result: %T", fresh)}
		}

		return DependenciesUpdatedMsg{
			Updated:      len(updatable),
			Dependencies: []ModuleDependency(depsMsg),
		}
	}
}

// ListModuleDependencies lists current module dependencies
// without checking for updates online.
func ListModuleDependencies(moduleDir string) tea.Cmd {
	return func() tea.Msg {
		return loadDependencies(moduleDir, false)
	}
}

// CheckModuleDependencyUpdates lists module dependencies
// and checks for available updates online.
func CheckModuleDependencyUpdates(moduleDir string) tea.Cmd {
	return func() tea.Msg {
		return loadDependencies(moduleDir, true)
	}
}

func loadDependencies(moduleDir string, checkUpdates bool) tea.Msg {
	args := []string{"list", "-mod=readonly", "-m", "-json"}
	if checkUpdates {
		args = append(args, "-u")
	}
	args = append(args, "all")

	cmd := exec.Command("go", args...)
	cmd.Dir = moduleDir

	output, err := cmd.Output()
	if err != nil {
		exitErr, ok := err.(*exec.ExitError)
		if ok && len(exitErr.Stderr) > 0 {
			return DependencyErrMsg{Err: fmt.Errorf("go list failed: %s", string(exitErr.Stderr))}
		}
		return DependencyErrMsg{Err: fmt.Errorf("go list failed: %w", err)}
	}

	dec := json.NewDecoder(strings.NewReader(string(output)))
	var deps []ModuleDependency

	for dec.More() {
		var raw struct {
			Path       string
			Version    string
			Main       bool
			Indirect   bool
			Deprecated string
			Error      *struct {
				Err string
			}
			Update *struct {
				Version string
			}
		}
		if err := dec.Decode(&raw); err != nil {
			return DependencyErrMsg{Err: fmt.Errorf("failed to parse go list output: %w", err)}
		}

		// Skip the main module itself.
		if raw.Main {
			continue
		}

		d := ModuleDependency{
			Path:     raw.Path,
			Version:  raw.Version,
			Indirect: raw.Indirect,
		}

		if raw.Deprecated != "" {
			d.Deprecated = raw.Deprecated
		}

		if raw.Error != nil {
			d.Error = raw.Error.Err
		}

		if raw.Update != nil {
			d.Latest = raw.Update.Version
		}

		deps = append(deps, d)
	}

	return DependenciesMsg(deps)
}
