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
			root := t.TempDir()
			writeFile(t, root, "go.mod", "module example.com/module\n\ngo 1.26\n")
			want := []ModuleDependency{{
				Path:    "example.com/dependency",
				Version: "v1.0.0",
				Latest:  "v1.1.0",
			}}
			loaded := false
			got, err := listModuleDependencies(root, tt.checkUpdates, dependencyOperation{
				load: func(ctx moduleContext, checkUpdates bool) ([]ModuleDependency, error) {
					loaded = true
					if ctx.Root != root {
						t.Fatalf("root = %q, want %q", ctx.Root, root)
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
			if !loaded {
				t.Fatalf("loaded = %t, want true", loaded)
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("dependencies = %#v, want %#v", got, want)
			}
		})
	}
}

func TestListModuleDependencies_ReturnsTypedErrors(t *testing.T) {
	loadErr := errors.New("load failed")
	tests := []struct {
		name      string
		operation dependencyOperation
		wantErr   error
	}{
		{
			name: "load error",
			operation: dependencyOperation{
				load: func(_ moduleContext, _ bool) ([]ModuleDependency, error) {
					return nil, loadErr
				},
			},
			wantErr: loadErr,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			writeFile(t, root, "go.mod", "module example.com/module\n\ngo 1.26\n")
			_, err := listModuleDependencies(root, false, tt.operation)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestRunModuleDependencyChecks_Success(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "go.mod", "module example.com/module\n\ngo 1.26\n")
	calls := [][]string{}
	result, err := runModuleDependencyChecks(root, dependencyOperation{
		runCommand: func(ctx moduleContext, args ...string) ([]byte, error) {
			if ctx.Root != root {
				t.Fatalf("root = %q, want %q", ctx.Root, root)
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
	root := t.TempDir()
	writeFile(t, root, "go.mod", "module example.com/module\n\ngo 1.26\n")
	calls := 0
	const maxCheckOutputLines = 8
	output := strings.Repeat("failure\n", maxCheckOutputLines+2)
	result, err := runModuleDependencyChecks(root, dependencyOperation{
		runCommand: func(_ moduleContext, _ ...string) ([]byte, error) {
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
	// Test removed: resolution now happens outside runModuleDependencyChecks
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
			saveBackup: func(_ moduleContext, _ *DependencySnapshot, _ string, _ int) (DependencyBackupInfo, error) {
				return DependencyBackupInfo{}, nil
			},
			runCommand: func(ctx moduleContext, _ ...string) ([]byte, error) {
				commandCalls++
				if commandCalls == 1 {
					if err := os.WriteFile(filepath.Join(ctx.Root, "go.mod"), []byte(updatedMod), 0644); err != nil {
						t.Fatalf("write updated go.mod: %v", err)
					}
					return nil, nil
				}
				return []byte("tidy failed"), errors.New("exit status 1")
			},
			load: func(_ moduleContext, _ bool) ([]ModuleDependency, error) {
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
						saveBackup: func(_ moduleContext, _ *DependencySnapshot, _ string, _ int) (DependencyBackupInfo, error) {
							return DependencyBackupInfo{}, nil
						},
						runCommand: func(_ moduleContext, _ ...string) ([]byte, error) {
							return nil, nil
						},
						load: func(_ moduleContext, _ bool) ([]ModuleDependency, error) {
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
					restoreFiles: func(_ moduleContext, _ *DependencySnapshot) error {
						return nil
					},
					load: func(_ moduleContext, _ bool) ([]ModuleDependency, error) {
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
					restoreFiles: func(_ moduleContext, _ *DependencySnapshot) error {
						return nil
					},
					load: func(_ moduleContext, _ bool) ([]ModuleDependency, error) {
						return nil, refreshErr
					},
					saveBackup: func(_ moduleContext, _ *DependencySnapshot, _ string, _ int) (DependencyBackupInfo, error) {
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
