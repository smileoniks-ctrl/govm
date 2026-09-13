package model

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/smileoniks-ctrl/govm/internal/deps"
	"github.com/smileoniks-ctrl/govm/internal/styles"
)

// depsDialogKind identifies which Yes/No dependency dialog is currently
// active. The zero value dialogIdle means "no dialog open", which
// makes a freshly constructed depsDialog inactive by default.
type depsDialogKind int

const (
	dialogIdle depsDialogKind = iota
	dialogUpdate
	dialogChecks
	dialogRollback
	dialogRestore
)

// dialogAction is the side-effect-free signal returned by
// depsDialog.Handle. Model.Update interprets it to decide which
// per-kind side-effect runner to invoke (apply*Choice / cancel*).
type dialogAction int

const (
	dialogNoop        dialogAction = iota // ←/→, ↑/↓ — state already updated inside Handle
	dialogConfirm                         // enter / y — caller runs the per-kind confirm path
	dialogCancel                          // n / esc — caller runs the per-kind cancel path
	dialogChangeLevel                     // ↑/↓ on the update dialog — Level already updated; caller rebuilds the plan
	dialogChangeScope                     // space on the update dialog — Explicit already flipped; caller rebuilds the plan
)

// depsDialog is the single module that owns the active Yes/No
// dialog for the Deps tab. Four dialogs (update, checks, rollback,
// restore) collapse into one struct parameterised by Kind. Only
// restore uses the Cursor / MaxCursor pair, which controls navigation
// over the Backups slice stored on depsTab. UpdateEntries and
// CheckResult retain the prompt payload so rendering stays independent
// from the cycle's defensively copied accessors. Inconclusive selects
// the distinct rollback copy used when checks could not run.
//
// The struct is intentionally small and side-effect free. Key handling
// that mutates only dialog-internal state (choice toggle, list
// navigation) lives in Handle; commands and depsTab mutations stay
// in Model.Update, which interprets the returned dialogAction.
type depsDialog struct {
	kind          depsDialogKind
	choiceYes     bool
	cursor        int
	maxCursor     int
	inconclusive  bool
	updateEntries []deps.DependencyUpdateEntry
	checkResult   *deps.DependencyCheckResult
	// Level is the Update level the update dialog's entries were built
	// for; Explicit is true when the plan covers ExplicitModules (the
	// Update scope "marked"/"current") rather than every direct
	// dependency. ExplicitModules is captured when the dialog opens so
	// the scope can be toggled back and forth without re-reading marks.
	level           deps.UpdateLevel
	explicit        bool
	explicitModules []string
}

// CanToggleScope reports whether the update dialog has an explicit
// module set to switch to.
func (d depsDialog) canToggleScope() bool {
	return d.kind == dialogUpdate && len(d.explicitModules) > 0
}

// Active reports whether any dialog is currently open. The zero value
// of depsDialog (Kind == dialogIdle) is inactive.
func (d depsDialog) active() bool { return d.kind != dialogIdle }

// Handle translates a key press into a new dialog state plus an
// action the caller interprets. It mutates only fields on the dialog
// itself (ChoiceYes, Cursor). Per-kind commands, status messages, and
// in-flight flag mutations are performed by the caller based on the
// returned action.
func (d depsDialog) handle(msg tea.KeyPressMsg) (depsDialog, dialogAction) {
	// The update dialog uses the vertical keys to cycle the level and
	// space to toggle the scope between all direct dependencies and
	// the explicit set.
	if d.kind == dialogUpdate {
		switch msg.String() {
		case "up", "k":
			d.level = adjacentLevel(d.level, -1)
			return d, dialogChangeLevel
		case "down", "j":
			d.level = adjacentLevel(d.level, +1)
			return d, dialogChangeLevel
		case "space":
			if !d.canToggleScope() {
				return d, dialogNoop
			}
			d.explicit = !d.explicit
			return d, dialogChangeScope
		}
	}
	// Restore is the only kind that navigates a list inside the dialog.
	if d.kind == dialogRestore {
		switch msg.String() {
		case "up", "k":
			if d.cursor > 0 {
				d.cursor--
			}
			return d, dialogNoop
		case "down", "j":
			if d.cursor < d.maxCursor {
				d.cursor++
			}
			return d, dialogNoop
		}
	}

	var action dialogAction
	d.choiceYes, action = yesNoKeyAction(msg.String(), d.choiceYes)
	return d, action
}

// yesNoKeyAction is the key handling every Yes/No dialog shares: the
// horizontal keys toggle the highlighted button, enter commits it, y
// and n answer directly, esc declines. It returns the new choice and
// the action the caller enacts. dialogConfirm is returned only when
// the committed choice is Yes; enter on No cancels.
func yesNoKeyAction(key string, choiceYes bool) (bool, dialogAction) {
	switch key {
	case "left", "right", "tab", "shift+tab", "h", "l":
		return !choiceYes, dialogNoop
	case "enter":
		if choiceYes {
			return choiceYes, dialogConfirm
		}
		return choiceYes, dialogCancel
	case "y", "Y":
		return true, dialogConfirm
	case "n", "N", "esc":
		return false, dialogCancel
	}
	return choiceYes, dialogNoop
}

// Render composes the dialog's body (kind-specific), the shared Yes/No
// buttons, and the outer renderDialog wrapper. The theme is taken as a
// parameter so depsDialog has no hidden dependency on package-level
// style state. Callers only need to overlay the returned string onto
// the active view.
func (d depsDialog) render(t styles.Theme, tab depsTab, viewport viewportSize) string {
	lines := d.bodyLines(t, tab)
	lines = append(lines, "")
	lines = append(lines, d.renderButtons(t))
	return renderDialog(t, lipgloss.JoinVertical(lipgloss.Left, lines...), d.errorStyle(), viewport)
}

func (d depsDialog) renderButtons(t styles.Theme) string {
	yesLabel, noLabel := buttonLabels(d.kind)
	return renderYesNoButtons(t, d.choiceYes, yesLabel, noLabel)
}

// renderYesNoButtons draws the shared button row of a Yes/No dialog
// with the chosen button highlighted.
func renderYesNoButtons(t styles.Theme, choiceYes bool, yesLabel, noLabel string) string {
	yesBtn, noBtn := t.DialogInactiveStyle, t.DialogInactiveStyle
	if choiceYes {
		yesBtn = t.DialogActiveStyle
	} else {
		noBtn = t.DialogActiveStyle
	}
	return lipgloss.JoinHorizontal(lipgloss.Center,
		yesBtn.Render(yesLabel),
		"  ",
		noBtn.Render(noLabel),
	)
}

func (d depsDialog) errorStyle() bool {
	return d.kind == dialogRollback
}

func buttonLabels(kind depsDialogKind) (yes, no string) {
	switch kind {
	case dialogRollback:
		return "Roll back", "Keep"
	case dialogRestore:
		return "Restore", "Cancel"
	default:
		return "Yes", "No"
	}
}

func (d depsDialog) bodyLines(t styles.Theme, tab depsTab) []string {
	switch d.kind {
	case dialogUpdate:
		return updateDialogLines(t, d.updateEntries, d.level, d.explicit, explicitScopeLabel(tab, d.explicitModules))
	case dialogChecks:
		return checksDialogLines(t)
	case dialogRollback:
		return rollbackDialogLines(t, d.checkResult, d.inconclusive)
	case dialogRestore:
		return restoreDialogLines(t, tab.backups, d.cursor)
	}
	return nil
}

// adjacentLevel returns the level step positions away from level in
// deps.Levels order, wrapping at both ends.
func adjacentLevel(level deps.UpdateLevel, step int) deps.UpdateLevel {
	n := len(deps.Levels)
	for i, l := range deps.Levels {
		if l == level {
			return deps.Levels[((i+step)%n+n)%n]
		}
	}
	return deps.Levels[0]
}

func updateDialogLines(t styles.Theme, updatable []deps.DependencyUpdateEntry, level deps.UpdateLevel, explicit bool, explicitLabel string) []string {
	lines := make([]string, 0, 9+len(updatable))
	lines = append(lines, t.DialogTitleStyle.Render(t.DialogWarningStyle.Render("⚠ Warning")))
	lines = append(lines, "")
	lines = append(lines, levelSelectorLine(t, level))
	lines = append(lines, scopeSelectorLine(t, explicit, explicitLabel))
	lines = append(lines, "")
	if len(updatable) == 0 {
		lines = append(lines, t.DialogBodyStyle.Render(fmt.Sprintf(
			"No updates available at the %s level.", level,
		)))
		lines = append(lines, "")
		hint := "↑/↓ change level · Yes ends without changes"
		if explicitLabel != "" {
			hint = "↑/↓ change level · space change scope · Yes ends without changes"
		}
		lines = append(lines, t.DialogMutedStyle.Render(hint))
		return lines
	}
	kind := "direct "
	if explicit {
		kind = ""
	}
	lines = append(lines, t.DialogBodyStyle.Render(fmt.Sprintf(
		"%d %s%s will be updated:",
		len(updatable),
		kind,
		deps.Pluralize(len(updatable), "dependency", "dependencies"),
	)))

	visible := updatable
	extra := 0
	if len(visible) > maxDependencyListLines {
		extra = len(visible) - maxDependencyListLines
		visible = visible[:maxDependencyListLines]
	}
	for _, e := range visible {
		lines = append(lines, t.DialogBodyStyle.Render(fmt.Sprintf(
			"  %s: %s -> %s", e.Path, e.OldVersion, e.NewVersion,
		)))
	}
	if extra > 0 {
		lines = append(lines, t.DialogBodyStyle.Render(
			fmt.Sprintf("  …and %d more", extra),
		))
	}
	lines = append(lines, "")
	lines = append(lines, t.DialogBodyStyle.Render("go.mod and go.sum will be modified."))
	lines = append(lines, t.DialogBodyStyle.Render("A snapshot is taken before the update so changes can be rolled back."))
	return lines
}

// levelSelectorLine renders "Level: Patch  Minor  [Latest]" with the
// active level highlighted.
func levelSelectorLine(t styles.Theme, level deps.UpdateLevel) string {
	parts := make([]string, 0, len(deps.Levels))
	for _, l := range deps.Levels {
		if l == level {
			parts = append(parts, t.DialogActiveStyle.Render(l.Label()))
		} else {
			parts = append(parts, t.DialogMutedStyle.Render(l.Label()))
		}
	}
	return lipgloss.JoinHorizontal(lipgloss.Center,
		t.DialogBodyStyle.Render("Level:"),
		lipgloss.JoinHorizontal(lipgloss.Center, parts...),
	)
}

// scopeSelectorLine renders "Scope: All  [Marked (2)]" with the active
// Update scope highlighted. Without an explicit set (no marks and no
// cursor module) only "All" is shown.
func scopeSelectorLine(t styles.Theme, explicit bool, explicitLabel string) string {
	styleFor := func(active bool) lipgloss.Style {
		if active {
			return t.DialogActiveStyle
		}
		return t.DialogMutedStyle
	}
	parts := []string{styleFor(!explicit).Render("All")}
	if explicitLabel != "" {
		parts = append(parts, styleFor(explicit).Render(explicitLabel))
	}
	return lipgloss.JoinHorizontal(lipgloss.Center,
		t.DialogBodyStyle.Render("Scope:"),
		lipgloss.JoinHorizontal(lipgloss.Center, parts...),
	)
}

// explicitScopeLabel names the explicit Update scope offered by the
// dialog: the marked modules when marks exist, otherwise the module
// under the cursor. Empty when there is no explicit set.
func explicitScopeLabel(state depsTab, modules []string) string {
	if len(modules) == 0 {
		return ""
	}
	if n := len(state.markedPaths()); n > 0 {
		return fmt.Sprintf("Marked (%d)", n)
	}
	return "Current"
}

func checksDialogLines(t styles.Theme) []string {
	return []string{
		t.DialogTitleStyle.Render(t.StatusInfoStyle.Render("✓ Run checks?")),
		"",
		t.DialogBodyStyle.Render("After the update the following will be executed:"),
		t.DialogBodyStyle.Render("  • go test ./..."),
		t.DialogBodyStyle.Render("  • go vet ./..."),
		"",
		t.DialogMutedStyle.Render("If a check fails you will be offered to roll back the dependencies."),
	}
}

func rollbackDialogLines(t styles.Theme, result *deps.DependencyCheckResult, inconclusive bool) []string {
	title := "⚠ Checks failed"
	if inconclusive {
		title = "⚠ Checks inconclusive"
	}
	lines := []string{
		t.DialogTitleStyle.Render(t.DialogWarningStyle.Render(title)),
		"",
	}
	if result != nil && result.Command != "" {
		lines = append(lines, t.DialogBodyStyle.Render(fmt.Sprintf("Command: %s", result.Command)))
		if result.Output != "" {
			output := strings.Split(result.Output, "\n")
			visible := output
			if len(visible) > maxDependencyListLines {
				visible = visible[:maxDependencyListLines]
			}
			for _, l := range visible {
				lines = append(lines, t.DialogMutedStyle.Render(l))
			}
			if extra := len(output) - len(visible); extra > 0 {
				lines = append(lines, t.DialogMutedStyle.Render(fmt.Sprintf("…and %d more", extra)))
			}
		}
		lines = append(lines, "")
	}
	lines = append(lines, t.DialogBodyStyle.Render("Roll back the dependencies to their pre-update state?"))
	return lines
}

func restoreDialogLines(t styles.Theme, backups []deps.DependencyBackupInfo, cursor int) []string {
	lines := []string{
		t.DialogTitleStyle.Render(t.DialogWarningStyle.Render("Dependency backups")),
		"",
		t.DialogBodyStyle.Render("Choose a saved dependency backup:"),
	}
	start := 0
	if cursor >= maxDependencyListLines {
		start = cursor - maxDependencyListLines + 1
	}
	if maxStart := len(backups) - maxDependencyListLines; start > maxStart {
		start = maxStart
	}
	if start < 0 {
		start = 0
	}
	end := len(backups)
	if end > start+maxDependencyListLines {
		end = start + maxDependencyListLines
	}
	visible := backups[start:end]
	for i, b := range visible {
		prefix := "  "
		if start+i == cursor {
			prefix = "> "
		}
		lines = append(lines, t.DialogBodyStyle.Render(fmt.Sprintf(
			"%s%s  %s  %d update(s)",
			prefix,
			b.Name,
			b.Kind,
			b.Updated,
		)))
	}
	if len(backups) > end {
		lines = append(lines, t.DialogBodyStyle.Render(fmt.Sprintf("  …and %d more", len(backups)-end)))
	}
	lines = append(lines, "")
	lines = append(lines, t.DialogMutedStyle.Render("Current go.mod and go.sum will be saved before restore."))
	return lines
}
