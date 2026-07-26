package model

import (
	"context"
	"errors"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/smileoniks-ctrl/govm/internal/prune"
	"github.com/smileoniks-ctrl/govm/internal/services"
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

func PreviewPruneCmd(rt *services.Runtime) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if rt == nil || rt.Prune == nil {
			return prunePreviewMsg{Err: errors.New("no prune service configured")}
		}
		result, err := rt.Prune.Preview(ctx)
		return prunePreviewMsg{Result: result, Err: err}
	}
}

func PruneCmd(rt *services.Runtime) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()
		if rt == nil || rt.Prune == nil {
			return pruneDoneMsg{Err: errors.New("no prune service configured")}
		}
		result, err := rt.Prune.Prune(ctx)
		return pruneDoneMsg{Result: result, Err: err}
	}
}

func DiskUsageCmd(rt *services.Runtime) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if rt == nil || rt.Prune == nil {
			return diskUsageMsg{Err: errors.New("no prune service configured")}
		}
		summary, err := rt.Prune.DiskUsage(ctx)
		return diskUsageMsg{Summary: summary, Err: err}
	}
}

func (m Model) previewPruneCmd() tea.Cmd {
	return func() tea.Msg {
		if m.previewPrune == nil {
			return prunePreviewMsg{Err: errors.New("no prune service configured")}
		}
		result, err := m.previewPrune(context.Background())
		return prunePreviewMsg{Result: result, Err: err}
	}
}

func (m Model) pruneCmd() tea.Cmd {
	return func() tea.Msg {
		if m.prune == nil {
			return pruneDoneMsg{Err: errors.New("no prune service configured")}
		}
		result, err := m.prune(context.Background())
		return pruneDoneMsg{Result: result, Err: err}
	}
}
