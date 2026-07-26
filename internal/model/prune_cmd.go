package model

import (
	"context"
	"errors"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/smileoniks-ctrl/govm/internal/prune"
)

// Wall-clock budgets granted to a single prune operation. Preview and
// disk usage only walk the managed directories, while a prune waits for
// the shared mutation lock and then removes toolchains.
const (
	prunePreviewTimeout = 30 * time.Second
	pruneTimeout        = 30 * time.Minute
	diskUsageTimeout    = 30 * time.Second
)

type previewPruneFunc func(context.Context) (prune.Result, error)
type pruneFunc func(context.Context) (prune.Result, error)
type diskUsageFunc func(context.Context) (prune.Summary, error)

type prunePreviewMsg struct {
	Result prune.Result
	Err    error
}

type pruneDoneMsg struct {
	Result prune.Result
	Err    error
}

type diskUsageMsg struct {
	Summary prune.Summary
	Err     error
}

func (m Model) previewPruneCmd() tea.Cmd {
	return func() tea.Msg {
		if m.previewPrune == nil {
			return prunePreviewMsg{Err: errors.New("no prune service configured")}
		}
		ctx, cancel := context.WithTimeout(context.Background(), prunePreviewTimeout)
		defer cancel()
		result, err := m.previewPrune(ctx)
		return prunePreviewMsg{Result: result, Err: err}
	}
}

func (m Model) pruneCmd() tea.Cmd {
	return func() tea.Msg {
		if m.prune == nil {
			return pruneDoneMsg{Err: errors.New("no prune service configured")}
		}
		ctx, cancel := context.WithTimeout(context.Background(), pruneTimeout)
		defer cancel()
		result, err := m.prune(ctx)
		return pruneDoneMsg{Result: result, Err: err}
	}
}

func (m Model) diskUsageCmd() tea.Cmd {
	if m.diskUsage == nil {
		return nil
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), diskUsageTimeout)
		defer cancel()
		summary, err := m.diskUsage(ctx)
		return diskUsageMsg{Summary: summary, Err: err}
	}
}
