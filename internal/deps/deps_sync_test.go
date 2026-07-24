package deps

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestListModuleDependencies_DelegatesToTypedLoader(t *testing.T) {
	tests := []struct {
		name         string
		checkUpdates bool
	}{
		{
			name:         "list without update check",
			checkUpdates: false,
		},
		{
			name:         "check available updates",
			checkUpdates: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			want := []ModuleDependency{{
				Path:    "example.com/dependency",
				Version: "v1.0.0",
				Latest:  "v1.1.0",
			}}
			resolved := false
			loaded := false
			got, err := listModuleDependencies("nested/module", tt.checkUpdates, dependencyOperation{
				resolveRoot: func(moduleDir string) (string, error) {
					resolved = true
					if moduleDir != "nested/module" {
						t.Fatalf("module dir = %q, want %q", moduleDir, "nested/module")
					}
					return "/module/root", nil
				},
				load: func(root string, checkUpdates bool) ([]ModuleDependency, error) {
					loaded = true
					if root != "/module/root" {
						t.Fatalf("root = %q, want %q", root, "/module/root")
					}
					if checkUpdates != tt.checkUpdates {
						t.Fatalf("check updates = %t, want %t", checkUpdates, tt.checkUpdates)
					}
					return want, nil
				},
			})
			if err != nil {
				t.Fatalf("listModuleDependencies: %v", err)
			}
			if !resolved || !loaded {
				t.Fatalf("resolved = %t, loaded = %t, want both true", resolved, loaded)
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("dependencies = %#v, want %#v", got, want)
			}
		})
	}
}

func TestListModuleDependencies_ReturnsTypedErrors(t *testing.T) {
	resolveErr := errors.New("resolve failed")
	loadErr := errors.New("load failed")
	tests := []struct {
		name      string
		operation dependencyOperation
		wantErr   error
	}{
		{
			name: "resolve error",
			operation: dependencyOperation{
				resolveRoot: func(string) (string, error) {
					return "", resolveErr
				},
			},
			wantErr: resolveErr,
		},
		{
			name: "load error",
			operation: dependencyOperation{
				resolveRoot: func(string) (string, error) {
					return "/module/root", nil
				},
				load: func(string, bool) ([]ModuleDependency, error) {
					return nil, loadErr
				},
			},
			wantErr: loadErr,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := listModuleDependencies("module", false, tt.operation)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestRunModuleDependencyChecks_Success(t *testing.T) {
	calls := [][]string{}
	result, err := runModuleDependencyChecks("module", dependencyOperation{
		resolveRoot: func(string) (string, error) {
			return "/module/root", nil
		},
		runCommand: func(root string, args ...string) ([]byte, error) {
			if root != "/module/root" {
				t.Fatalf("root = %q, want %q", root, "/module/root")
			}
			calls = append(calls, append([]string{}, args...))
			return nil, nil
		},
	})
	if err != nil {
		t.Fatalf("runModuleDependencyChecks: %v", err)
	}
	if !result.OK {
		t.Fatalf("result = %+v, want OK", result)
	}
	wantCalls := [][]string{{"test", "./..."}, {"vet", "./..."}}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("commands = %#v, want %#v", calls, wantCalls)
	}
}

func TestRunModuleDependencyChecks_CommandFailureIsResult(t *testing.T) {
	calls := 0
	output := strings.Repeat("failure\n", maxCheckOutputLines+2)
	result, err := runModuleDependencyChecks("module", dependencyOperation{
		resolveRoot: func(string) (string, error) {
			return "/module/root", nil
		},
		runCommand: func(string, ...string) ([]byte, error) {
			calls++
			return []byte(output), errors.New("exit status 1")
		},
	})
	if err != nil {
		t.Fatalf("runModuleDependencyChecks: %v", err)
	}
	if result.OK || result.Command != "go test ./..." {
		t.Fatalf("result = %+v, want failed go test result", result)
	}
	if !strings.Contains(result.Output, "more lines") {
		t.Fatalf("output = %q, want trimmed output", result.Output)
	}
	if calls != 1 {
		t.Fatalf("command calls = %d, want 1", calls)
	}
}

func TestRunModuleDependencyChecks_ResolveError(t *testing.T) {
	resolveErr := errors.New("resolve failed")
	_, err := runModuleDependencyChecks("module", dependencyOperation{
		resolveRoot: func(string) (string, error) {
			return "", resolveErr
		},
		runCommand: func(string, ...string) ([]byte, error) {
			t.Fatal("command must not run after resolve error")
			return nil, nil
		},
	})
	if !errors.Is(err, resolveErr) {
		t.Fatalf("error = %v, want %v", err, resolveErr)
	}
}

func TestUpdateModuleDependencies_NoEntries(t *testing.T) {
	_, err := UpdateModuleDependencies("module", nil, 3)
	if err == nil || err.Error() != "no direct dependency updates available" {
		t.Fatalf("error = %v, want no-updates error", err)
	}
}

func TestUpdateModuleDependencies_TidyFailureLeavesUpdatedFiles(t *testing.T) {
	root := t.TempDir()
	originalMod := "module example.com/app\n\ngo 1.26\n"
	updatedMod := originalMod + "\nrequire example.com/dependency v1.1.0\n"
	writeFile(t, root, "go.mod", originalMod)

	commandCalls := 0
	_, _, _, err := applyModuleUpdates(
		root,
		[]DependencyUpdateEntry{{
			Path:       "example.com/dependency",
			OldVersion: "v1.0.0",
			NewVersion: "v1.1.0",
		}},
		3,
		dependencyOperation{
			resolveContext: func(string) (moduleContext, error) {
				return moduleContext{Root: root, Path: "example.com/app"}, nil
			},
			saveBackup: func(moduleContext, *DependencySnapshot, string, int) (DependencyBackupInfo, error) {
				return DependencyBackupInfo{}, nil
			},
			runCommand: func(string, ...string) ([]byte, error) {
				commandCalls++
				if commandCalls == 1 {
					if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte(updatedMod), 0644); err != nil {
						t.Fatalf("write updated go.mod: %v", err)
					}
					return nil, nil
				}
				return []byte("tidy failed"), errors.New("exit status 1")
			},
			load: func(string, bool) ([]ModuleDependency, error) {
				t.Fatal("loader must not run after tidy error")
				return nil, nil
			},
		},
	)
	if err == nil || !strings.Contains(err.Error(), "go mod tidy failed: tidy failed") {
		t.Fatalf("error = %v, want contextual tidy error", err)
	}
	got, readErr := os.ReadFile(filepath.Join(root, "go.mod"))
	if readErr != nil {
		t.Fatalf("read go.mod: %v", readErr)
	}
	if string(got) != updatedMod {
		t.Fatalf("go.mod = %q, want updated bytes %q", got, updatedMod)
	}
}

func TestDependencyMutationRefreshErrors(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "go.mod", "module example.com/app\n\ngo 1.26\n")
	refreshErr := errors.New("refresh failed")
	snapshot := &DependencySnapshot{
		ModFile: ModuleFileSnapshot{
			Exists:  true,
			Content: "module example.com/app\n\ngo 1.26\n",
		},
	}

	tests := []struct {
		name string
		run  func() error
	}{
		{
			name: "update",
			run: func() error {
				_, _, _, err := applyModuleUpdates(
					root,
					[]DependencyUpdateEntry{{
						Path:       "example.com/dependency",
						OldVersion: "v1.0.0",
						NewVersion: "v1.1.0",
					}},
					3,
					dependencyOperation{
						resolveContext: func(string) (moduleContext, error) {
							return moduleContext{Root: root, Path: "example.com/app"}, nil
						},
						saveBackup: func(moduleContext, *DependencySnapshot, string, int) (DependencyBackupInfo, error) {
							return DependencyBackupInfo{}, nil
						},
						runCommand: func(string, ...string) ([]byte, error) {
							return nil, nil
						},
						load: func(string, bool) ([]ModuleDependency, error) {
							return nil, refreshErr
						},
					},
				)
				return err
			},
		},
		{
			name: "rollback",
			run: func() error {
				_, err := rollbackModuleDependencies(root, snapshot, dependencyOperation{
					resolveRoot: func(string) (string, error) {
						return root, nil
					},
					restoreFiles: func(string, *DependencySnapshot) error {
						return nil
					},
					runCommand: func(string, ...string) ([]byte, error) {
						return nil, nil
					},
					load: func(string, bool) ([]ModuleDependency, error) {
						return nil, refreshErr
					},
				})
				return err
			},
		},
		{
			name: "restore",
			run: func() error {
				_, err := restoreDependencyBackup(root, "saved.json", 3, dependencyOperation{
					resolveContext: func(string) (moduleContext, error) {
						return moduleContext{Root: root, Path: "example.com/app"}, nil
					},
					restoreFiles: func(string, *DependencySnapshot) error {
						return nil
					},
					runCommand: func(string, ...string) ([]byte, error) {
						return nil, nil
					},
					load: func(string, bool) ([]ModuleDependency, error) {
						return nil, refreshErr
					},
					saveBackup: func(moduleContext, *DependencySnapshot, string, int) (DependencyBackupInfo, error) {
						return DependencyBackupInfo{}, nil
					},
					loadBackup: func(moduleContext, string) (*DependencyBackup, error) {
						return &DependencyBackup{Snapshot: snapshot}, nil
					},
				})
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.run(); !errors.Is(err, refreshErr) {
				t.Fatalf("error = %v, want %v", err, refreshErr)
			}
		})
	}
}
