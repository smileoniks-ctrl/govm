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
	Result     lifecycle.ActivationResult
	ShimInPath bool
}

type deletionSuccessMsg lifecycle.DeletionResult

type lifecycleFailureMsg struct {
	Operation string
	Version   string
	Err       error
}

func (m Model) activateVersionCmd(version string) tea.Cmd {
	return func() tea.Msg {
		if m.activateGo == nil {
			return lifecycleFailureMsg{Operation: "switch", Version: version, Err: errors.New("no lifecycle activator configured")}
		}
		result, err := m.activateGo(context.Background(), version)
		if err != nil {
			return lifecycleFailureMsg{Operation: "switch", Version: version, Err: err}
		}
		shimInPath := false
		if m.shimInPath != nil {
			shimInPath = m.shimInPath()
		}
		return activationSuccessMsg{Result: result, ShimInPath: shimInPath}
	}
}

func (m Model) deleteVersionCmd(version string) tea.Cmd {
	return func() tea.Msg {
		if m.deleteGo == nil {
			return lifecycleFailureMsg{Operation: "delete", Version: version, Err: errors.New("no lifecycle deleter configured")}
		}
		result, err := m.deleteGo(context.Background(), version)
		if err != nil {
			return lifecycleFailureMsg{Operation: "delete", Version: version, Err: err}
		}
		return deletionSuccessMsg(result)
	}
}

func joinLifecycleWarnings(warnings []lifecycle.Warning) string {
	parts := make([]string, len(warnings))
	for i, warning := range warnings {
		parts[i] = warning.Error()
	}
	return strings.Join(parts, "; ")
}
