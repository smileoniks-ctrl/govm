package deps

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"golang.org/x/mod/modfile"
)

// moduleContext bundles a resolved module root with its declared
// module path. It is shared by the dependency operations and the
// backup store.
type moduleContext struct {
	Root string
	Path string
}

// ResolveModuleRoot returns the absolute path to the Go module root
// containing startDir. It runs `go env GOMOD` with cmd.Dir = startDir
// so the search walks up the directory tree from startDir. The
// resolved module root is filepath.Dir of the go.mod path reported
// by Go. When startDir is not inside a Go module (the command
// fails, the output is empty, or it points at os.DevNull) a wrapped
// error is returned mentioning startDir.
func ResolveModuleRoot(startDir string) (string, error) {
	cmd := exec.Command("go", "env", "GOMOD")
	cmd.Dir = startDir
	out, err := cmd.Output()
	if err != nil {
		exitErr, ok := err.(*exec.ExitError)
		if ok && len(exitErr.Stderr) > 0 {
			return "", fmt.Errorf("not in a Go module (%s): %s", startDir, strings.TrimSpace(string(exitErr.Stderr)))
		}
		return "", fmt.Errorf("not in a Go module (%s): %w", startDir, err)
	}
	gomod := strings.TrimSpace(string(out))
	if gomod == "" || gomod == os.DevNull {
		return "", fmt.Errorf("not in a Go module (%s)", startDir)
	}
	return filepath.Dir(gomod), nil
}

func resolveModuleContext(moduleDir string) (moduleContext, error) {
	root, err := ResolveModuleRoot(moduleDir)
	if err != nil {
		return moduleContext{}, err
	}
	bytes, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		return moduleContext{}, fmt.Errorf("read go.mod: %w", err)
	}
	modulePath := modfile.ModulePath(bytes)
	if modulePath == "" {
		return moduleContext{}, fmt.Errorf("read go.mod: module path not found")
	}
	return moduleContext{Root: root, Path: modulePath}, nil
}

// SnapshotModuleFiles reads go.mod and go.sum from moduleDir and
// returns a snapshot of their current contents. It does not run any
// external command. Returns an error if go.mod is missing, since
// rolling back requires at least the module declaration.
func SnapshotModuleFiles(moduleDir string) (*DependencySnapshot, error) {
	snap := &DependencySnapshot{}

	modPath := filepath.Join(moduleDir, "go.mod")
	modBytes, err := os.ReadFile(modPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("snapshot: go.mod not found in %s", moduleDir)
		}
		return nil, fmt.Errorf("snapshot go.mod: %w", err)
	}
	snap.ModFile = ModuleFileSnapshot{Exists: true, Content: string(modBytes)}

	sumBytes, err := os.ReadFile(filepath.Join(moduleDir, "go.sum"))
	switch {
	case err == nil:
		snap.SumFile = ModuleFileSnapshot{Exists: true, Content: string(sumBytes)}
	case os.IsNotExist(err):
		snap.SumFile = ModuleFileSnapshot{Exists: false}
	default:
		return nil, fmt.Errorf("snapshot go.sum: %w", err)
	}

	return snap, nil
}

// RestoreModuleFiles writes snap.ModFile and snap.SumFile back to
// disk verbatim. If snap.SumFile.Exists is false, any existing go.sum
// is removed.
func RestoreModuleFiles(moduleDir string, snap *DependencySnapshot) error {
	if snap == nil {
		return fmt.Errorf("restore: nil snapshot")
	}
	if !snap.ModFile.Exists {
		return fmt.Errorf("restore: go.mod snapshot is missing")
	}

	if err := os.WriteFile(filepath.Join(moduleDir, "go.mod"), []byte(snap.ModFile.Content), 0644); err != nil {
		return fmt.Errorf("restore go.mod: %w", err)
	}

	sumPath := filepath.Join(moduleDir, "go.sum")
	if snap.SumFile.Exists {
		if err := os.WriteFile(sumPath, []byte(snap.SumFile.Content), 0644); err != nil {
			return fmt.Errorf("restore go.sum: %w", err)
		}
	} else {
		if err := os.Remove(sumPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove go.sum: %w", err)
		}
	}

	return nil
}
