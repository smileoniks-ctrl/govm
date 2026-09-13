package model

import (
	"errors"
	"reflect"
	"testing"
)

func TestDependencyCmd_ReturnsTypedMessage(t *testing.T) {
	want := dependenciesMsg{{Path: "example.com/dependency", Version: "v1.0.0"}}
	msg := dependencyCmd(func() (dependenciesMsg, error) {
		return want, nil
	})()

	got, ok := msg.(dependenciesMsg)
	if !ok {
		t.Fatalf("message = %T, want dependenciesMsg", msg)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("message = %#v, want %#v", got, want)
	}
}

func TestDependencyCmd_MapsError(t *testing.T) {
	wantErr := errors.New("dependency operation failed")
	msg := dependencyCmd(func() (dependenciesMsg, error) {
		return nil, wantErr
	})()

	errMsg, ok := msg.(dependencyErrMsg)
	if !ok {
		t.Fatalf("message = %T, want dependencyErrMsg", msg)
	}
	if !errors.Is(errMsg.Err, wantErr) {
		t.Fatalf("error = %v, want %v", errMsg.Err, wantErr)
	}
}

func TestDependencyCmd_PreservesBackupMessageType(t *testing.T) {
	want := dependencyBackupsMsg{{Name: "2026-07-09_12-00-00.json", Updated: 2}}
	msg := dependencyCmd(func() (dependencyBackupsMsg, error) {
		return want, nil
	})()

	got, ok := msg.(dependencyBackupsMsg)
	if !ok {
		t.Fatalf("message = %T, want dependencyBackupsMsg", msg)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("message = %#v, want %#v", got, want)
	}
}
