package model

import (
	"errors"
	"reflect"
	"testing"
)

func TestDependencyCmd_ReturnsTypedMessage(t *testing.T) {
	want := DependenciesMsg{{Path: "example.com/dependency", Version: "v1.0.0"}}
	msg := dependencyCmd(func() (DependenciesMsg, error) {
		return want, nil
	})()

	got, ok := msg.(DependenciesMsg)
	if !ok {
		t.Fatalf("message = %T, want DependenciesMsg", msg)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("message = %#v, want %#v", got, want)
	}
}

func TestDependencyCmd_MapsError(t *testing.T) {
	wantErr := errors.New("dependency operation failed")
	msg := dependencyCmd(func() (DependenciesMsg, error) {
		return nil, wantErr
	})()

	errMsg, ok := msg.(DependencyErrMsg)
	if !ok {
		t.Fatalf("message = %T, want DependencyErrMsg", msg)
	}
	if !errors.Is(errMsg.Err, wantErr) {
		t.Fatalf("error = %v, want %v", errMsg.Err, wantErr)
	}
}

func TestDependencyCmd_PreservesBackupMessageType(t *testing.T) {
	want := DependencyBackupsMsg{{Name: "2026-07-09_12-00-00.json", Updated: 2}}
	msg := dependencyCmd(func() (DependencyBackupsMsg, error) {
		return want, nil
	})()

	got, ok := msg.(DependencyBackupsMsg)
	if !ok {
		t.Fatalf("message = %T, want DependencyBackupsMsg", msg)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("message = %#v, want %#v", got, want)
	}
}
