package model

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/smileoniks-ctrl/govm/internal/styles"
	"github.com/smileoniks-ctrl/govm/internal/utils"
)

// DialogKind identifies which Yes/No dependency dialog is currently
// active. The zero value DialogIdle means "no dialog open", which
// makes a freshly constructed ConfirmDialog inactive by default.
type DialogKind int

const (
	DialogIdle DialogKind = iota
	DialogUpdate
	DialogChecks
	DialogRollback
	DialogRestore
)

// DialogAction is the side-effect-free signal returned by
// ConfirmDialog.Handle. Model.Update interprets it to decide which
// per-kind side-effect runner to invoke (apply*Choice / cancel*).
type DialogAction int

const (
	DialogNoop    DialogAction = iota // ←/→, ↑/↓ — state already updated inside Handle
	DialogConfirm                     // enter / y — caller runs the per-kind confirm path
	DialogCancel                      // n / esc — caller runs the per-kind cancel path
)

// ConfirmDialog is the single module that owns the active Yes/No
// dialog for the Deps tab. Four historical dialogs (update, checks,
// rollback, restore) collapse into one struct parameterised by Kind.
// Only restore uses the Cursor / MaxCursor pair, which controls
// navigation over the Backups slice stored on DepsState.
//
// The struct is intentionally small and side-effect free. Key handling
// that mutates only dialog-internal state (choice toggle, list
// navigation) lives in Handle; commands and DepsState mutations stay
// in Model.Update, which interprets the returned DialogAction.
type ConfirmDialog struct {
	Kind      DialogKind
	ChoiceYes bool
	Cursor    int
	MaxCursor int
}

// Active reports whether any dialog is currently open. The zero value
// of ConfirmDialog (Kind == DialogIdle) is inactive.
func (d ConfirmDialog) Active() bool { return d.Kind != DialogIdle }

// Handle translates a key press into a new dialog state plus an
// action the caller interprets. It mutates only fields on the dialog
// itself (ChoiceYes, Cursor). Per-kind commands, status messages, and
// in-flight flag mutations are performed by the caller based on the
// returned action.
func (d ConfirmDialog) Handle(msg tea.KeyPressMsg) (ConfirmDialog, DialogAction) {
	// Restore is the only kind that navigates a list inside the dialog.
	if d.Kind == DialogRestore {
		switch msg.String() {
		case "up", "k":
			if d.Cursor > 0 {
				d.Cursor--
			}
			return d, DialogNoop
		case "down", "j":
			if d.Cursor < d.MaxCursor {
				d.Cursor++
			}
			return d, DialogNoop
		}
	}

	switch msg.String() {
	case "left", "right", "tab", "h", "l":
		d.ChoiceYes = !d.ChoiceYes
		return d, DialogNoop
	case "enter":
		return d, DialogConfirm
	case "y", "Y":
		d.ChoiceYes = true
		return d, DialogConfirm
	case "n", "N", "esc":
		return d, DialogCancel
	}
	return d, DialogNoop
}

// Render composes the dialog's body (kind-specific), the shared Yes/No
// buttons, and the outer renderDialog wrapper. The theme is taken as a
// parameter so ConfirmDialog has no hidden dependency on package-level
// style state. Callers only need to overlay the returned string onto
// the active view.
func (d ConfirmDialog) Render(t styles.Theme, deps DepsState, viewport viewportSize) string {
	lines := d.bodyLines(t, deps)
	lines = append(lines, "")
	lines = append(lines, d.renderButtons(t))
	return renderDialog(t, lipgloss.JoinVertical(lipgloss.Left, lines...), d.errorStyle(), viewport)
}

func (d ConfirmDialog) renderButtons(t styles.Theme) string {
	yesBtn, noBtn := t.DialogInactiveStyle, t.DialogInactiveStyle
	if d.ChoiceYes {
		yesBtn = t.DialogActiveStyle
	} else {
		noBtn = t.DialogActiveStyle
	}
	yesLabel, noLabel := buttonLabels(d.Kind)
	return lipgloss.JoinHorizontal(lipgloss.Center,
		yesBtn.Render(yesLabel),
		"  ",
		noBtn.Render(noLabel),
	)
}

func (d ConfirmDialog) errorStyle() bool {
	return d.Kind == DialogRollback
}

func buttonLabels(kind DialogKind) (yes, no string) {
	switch kind {
	case DialogRollback:
		return "Roll back", "Keep"
	case DialogRestore:
		return "Restore", "Cancel"
	default:
		return "Yes", "No"
	}
}

func (d ConfirmDialog) bodyLines(t styles.Theme, deps DepsState) []string {
	switch d.Kind {
	case DialogUpdate:
		return updateDialogLines(t, deps.UpdateEntries)
	case DialogChecks:
		return checksDialogLines(t)
	case DialogRollback:
		return rollbackDialogLines(t, deps.LastCheckResult)
	case DialogRestore:
		return restoreDialogLines(t, deps.Backups, d.Cursor)
	}
	return nil
}

func updateDialogLines(t styles.Theme, updatable []utils.DependencyUpdateEntry) []string {
	lines := make([]string, 0, 6+len(updatable))
	lines = append(lines, t.DialogTitleStyle.Render(t.DialogWarningStyle.Render("⚠ Warning")))
	lines = append(lines, "")
	lines = append(lines, t.DialogBodyStyle.Render(fmt.Sprintf(
		"%d direct %s will be updated:",
		len(updatable),
		utils.Pluralize(len(updatable), "dependency", "dependencies"),
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

func rollbackDialogLines(t styles.Theme, result *utils.DependencyCheckResultMsg) []string {
	lines := []string{
		t.DialogTitleStyle.Render(t.DialogWarningStyle.Render("⚠ Checks failed")),
		"",
	}
	if result != nil {
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

func restoreDialogLines(t styles.Theme, backups []utils.DependencyBackupInfo, cursor int) []string {
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
