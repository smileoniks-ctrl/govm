package state

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func validMarker() Marker {
	return Marker{
		SchemaVersion: SchemaVersion,
		Operation:     OperationActivate,
		Phase:         "prepared",
		Version:       "1.26.1",
		Artifacts: map[string]string{
			"staging": ".activate-123",
			"backup":  ".shims-backup-123",
		},
	}
}

func TestMarkerValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(*Marker)
	}{
		{
			name:   "unknown schema",
			mutate: func(marker *Marker) { marker.SchemaVersion++ },
		},
		{
			name:   "missing operation",
			mutate: func(marker *Marker) { marker.Operation = "" },
		},
		{
			name:   "malformed operation",
			mutate: func(marker *Marker) { marker.Operation = "bad operation" },
		},
		{
			name:   "missing phase",
			mutate: func(marker *Marker) { marker.Phase = "" },
		},
		{
			name:   "malformed phase",
			mutate: func(marker *Marker) { marker.Phase = "PREPARED" },
		},
		{
			name:   "prefixed version",
			mutate: func(marker *Marker) { marker.Version = "go1.26.1" },
		},
		{
			name:   "unknown artifact role",
			mutate: func(marker *Marker) { marker.Artifacts["mystery"] = "safe-name" },
		},
		{
			name: "absolute artifact",
			mutate: func(marker *Marker) {
				marker.Artifacts["target"] = filepath.Join(string(filepath.Separator), "tmp", "target")
			},
		},
		{
			name:   "unix traversal artifact",
			mutate: func(marker *Marker) { marker.Artifacts["target"] = "../target" },
		},
		{
			name:   "windows traversal artifact",
			mutate: func(marker *Marker) { marker.Artifacts["target"] = `..\target` },
		},
		{
			name:   "nested artifact",
			mutate: func(marker *Marker) { marker.Artifacts["target"] = "versions/target" },
		},
		{
			name:   "dot artifact",
			mutate: func(marker *Marker) { marker.Artifacts["target"] = "." },
		},
		{
			name:   "empty artifact",
			mutate: func(marker *Marker) { marker.Artifacts["target"] = "" },
		},
	}

	if err := validMarker().Validate(); err != nil {
		t.Fatalf("valid marker rejected: %v", err)
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			marker := validMarker()
			tt.mutate(&marker)
			var markerErr *MarkerError
			if err := marker.Validate(); !errors.As(err, &markerErr) {
				t.Fatalf("Validate() error = %v, want *MarkerError", err)
			}
		})
	}
}

func TestMarkerStoreWriteReadDelete(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store := NewMarkerStore(root)
	marker := validMarker()

	if err := store.Write(marker); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	info, err := os.Stat(store.Path())
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("marker mode = %o, want 600", got)
	}

	got, present, err := store.Read()
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if !present {
		t.Fatal("Read() present = false, want true")
	}
	if !reflect.DeepEqual(got, marker) {
		t.Fatalf("Read() marker = %#v, want %#v", got, marker)
	}

	if err := store.Delete(); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if _, present, err := store.Read(); err != nil || present {
		t.Fatalf("Read() after Delete() = present %t, error %v", present, err)
	}
	if err := store.Delete(); err != nil {
		t.Fatalf("second Delete() error = %v", err)
	}
}

func TestMarkerStoreReadMissing(t *testing.T) {
	t.Parallel()

	marker, present, err := NewMarkerStore(t.TempDir()).Read()
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if present {
		t.Fatalf("Read() marker = %#v, present = true", marker)
	}
}

func TestMarkerStoreReadRejectsInvalidFiles(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		data string
	}{
		{name: "corrupt JSON", data: `{"schema_version":`},
		{name: "empty object", data: `{}`},
		{name: "unknown field", data: `{"schema_version":1,"operation":"install","phase":"prepared","version":"1.26.1","extra":true}`},
		{name: "trailing document", data: `{"schema_version":1,"operation":"install","phase":"prepared","version":"1.26.1"} {}`},
		{name: "unsafe artifact", data: `{"schema_version":1,"operation":"install","phase":"prepared","version":"1.26.1","artifacts":{"target":"../outside"}}`},
		{name: "windows unsafe artifact", data: `{"schema_version":1,"operation":"install","phase":"prepared","version":"1.26.1","artifacts":{"target":"..\\outside"}}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			store := NewMarkerStore(root)
			if err := os.WriteFile(store.Path(), []byte(tt.data), 0o600); err != nil {
				t.Fatal(err)
			}
			var markerErr *MarkerError
			if _, _, err := store.Read(); !errors.As(err, &markerErr) {
				t.Fatalf("Read() error = %v, want *MarkerError", err)
			}
		})
	}
}

func TestMarkerStoreWriteRejectsInvalidMarkerWithoutReplacingExisting(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store := NewMarkerStore(root)
	original := validMarker()
	if err := store.Write(original); err != nil {
		t.Fatal(err)
	}
	invalid := original
	invalid.Artifacts = map[string]string{"target": "../outside"}
	if err := store.Write(invalid); err == nil {
		t.Fatal("Write() error = nil, want validation failure")
	}
	got, present, err := store.Read()
	if err != nil || !present || !reflect.DeepEqual(got, original) {
		t.Fatalf("existing marker changed: got %#v, present %t, error %v", got, present, err)
	}
}

type failingMarkerFS struct {
	osMarkerFileSystem
	renameErr error
	removeErr error
}

func (fs failingMarkerFS) rename(_, _ string) error { return fs.renameErr }
func (fs failingMarkerFS) remove(string) error      { return fs.removeErr }

func TestMarkerStoreWriteRenameFailurePreservesOldMarker(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store := NewMarkerStore(root)
	original := validMarker()
	if err := store.Write(original); err != nil {
		t.Fatal(err)
	}
	store.fs = failingMarkerFS{renameErr: errors.New("rename failed")}
	replacement := validMarker()
	replacement.Phase = "committed"
	err := store.Write(replacement)
	if runtime.GOOS == "windows" {
		if err == nil || !strings.Contains(err.Error(), "replace transaction marker") {
			t.Fatalf("Write() error = %v", err)
		}
		return
	}
	if err == nil || !strings.Contains(err.Error(), "replace transaction marker") {
		t.Fatalf("Write() error = %v", err)
	}
	data, readErr := os.ReadFile(store.Path())
	if readErr != nil {
		t.Fatal(readErr)
	}
	if !strings.Contains(string(data), `"phase":"prepared"`) {
		t.Fatalf("existing marker was replaced: %s", data)
	}
}

func TestMarkerStoreWriteReplaceFailureDoesNotRemoveOldMarker(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store := NewMarkerStore(root)
	original := validMarker()
	if err := store.Write(original); err != nil {
		t.Fatal(err)
	}
	store.fs = &countingFailingMarkerFS{renameErr: errors.New("replace failed")}
	replacement := original
	replacement.Phase = "committed"
	if err := store.Write(replacement); err == nil {
		t.Fatal("Write() error = nil")
	}
	fs := store.fs.(*countingFailingMarkerFS)
	if fs.removeCalls != 0 {
		t.Fatalf("marker remove calls = %d, want 0", fs.removeCalls)
	}
	got, present, err := NewMarkerStore(root).Read()
	if err != nil || !present || !reflect.DeepEqual(got, original) {
		t.Fatalf("old marker = %#v, present %t, error %v", got, present, err)
	}
}

type countingFailingMarkerFS struct {
	osMarkerFileSystem
	renameErr   error
	removeCalls int
}

func (fs *countingFailingMarkerFS) rename(_, _ string) error { return fs.renameErr }
func (fs *countingFailingMarkerFS) remove(string) error {
	fs.removeCalls++
	return nil
}

func TestMarkerStoreSyncFailuresAreSurfaced(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store := NewMarkerStore(root)
	store.syncDir = func(string) error { return errors.New("sync failed") }
	if err := store.Write(validMarker()); err == nil || !strings.Contains(err.Error(), "sync transaction marker directory") {
		t.Fatalf("Write() error = %v", err)
	}
	if err := store.Delete(); err == nil || !strings.Contains(err.Error(), "sync transaction marker directory") {
		t.Fatalf("Delete() error = %v", err)
	}
}
