package cli

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

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
	return &DepsService{
		ModuleDir: moduleDir,
		Stdout:    stdout,
		Stdin:     stdin,
		Confirm:   defaultConfirm(stdin, stdout),
		ListDeps:  defaultListDeps,
		CheckDeps: defaultCheckDeps,
		Update:    defaultUpdate,
		RunChecks: defaultRunChecks,
		Rollback:  defaultRollback,
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

func defaultUpdate(moduleDir string, deps []utils.ModuleDependency) (utils.DependenciesUpdatedMsg, error) {
	msg := utils.UpdateModuleDependencies(moduleDir, deps)()
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

// defaultConfirm returns a Confirm implementation that prompts on
// stdout and reads a single line from stdin.
func defaultConfirm(stdin io.Reader, stdout io.Writer) func(string, bool) (bool, error) {
	return func(question string, defaultYes bool) (bool, error) {
		prompt := fmt.Sprintf("%s [%s]: ", question, yesNoLabel(defaultYes))
		for {
			if _, err := io.WriteString(stdout, prompt); err != nil {
				return false, err
			}
			scanner := bufio.NewScanner(stdin)
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
