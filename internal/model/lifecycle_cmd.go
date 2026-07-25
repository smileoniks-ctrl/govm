package model

import (
	"context"
	"errors"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/smileoniks-ctrl/govm/internal/lifecycle"
)

type activateFunc func(context.Context, string) (lifecycle.ActivationResult, error)
type deleteFunc func(context.Context, string) (lifecycle.DeletionResult, error)

type activationSuccessMsg struct {
	OperationID uint64
	Result      lifecycle.ActivationResult
	ShimInPath  bool
}

type deletionSuccessMsg struct {
	OperationID uint64
	Result      lifecycle.DeletionResult
}

type lifecycleFailureMsg struct {
	OperationID uint64
	Operation   string
	Version     string
	Err         error
}

func (m Model) activateVersionCmd(operationID uint64, version string) tea.Cmd {
	return func() tea.Msg {
		if m.activateGo == nil {
			return lifecycleFailureMsg{
				OperationID: operationID,
				Operation:   "switch",
				Version:     version,
				Err:         errors.New("no lifecycle activator configured"),
			}
		}
		result, err := m.activateGo(context.Background(), version)
		if err != nil {
			return lifecycleFailureMsg{
				OperationID: operationID,
				Operation:   "switch",
				Version:     version,
				Err:         err,
			}
		}
		shimInPath := false
		if m.shimInPath != nil {
			shimInPath = m.shimInPath()
		}
		return activationSuccessMsg{
			OperationID: operationID,
			Result:      result,
			ShimInPath:  shimInPath,
		}
	}
}

func (m Model) deleteVersionCmd(operationID uint64, version string) tea.Cmd {
	return func() tea.Msg {
		if m.deleteGo == nil {
			return lifecycleFailureMsg{
				OperationID: operationID,
				Operation:   "delete",
				Version:     version,
				Err:         errors.New("no lifecycle deleter configured"),
			}
		}
		result, err := m.deleteGo(context.Background(), version)
		if err != nil {
			return lifecycleFailureMsg{
				OperationID: operationID,
				Operation:   "delete",
				Version:     version,
				Err:         err,
			}
		}
		return deletionSuccessMsg{OperationID: operationID, Result: result}
	}
}

func joinLifecycleWarnings(warnings []lifecycle.Warning) string {
	parts := make([]string, len(warnings))
	for i, warning := range warnings {
		parts[i] = warning.Error()
	}
	return strings.Join(parts, "; ")
}
