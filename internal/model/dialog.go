package model

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/smileoniks-ctrl/govm/internal/styles"
	"github.com/smileoniks-ctrl/govm/internal/utils"
)

// Static styles for the dependency update dialogs. These are computed
// once at package init rather than on every render call, which
// previously recreated four `lipgloss.NewStyle()` values plus the
// wrapper border per View().
var (
	dialogTitleStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(styles.Warning).
				Padding(0, 1)

	dialogWarningStyle = styles.StatusWarningStyle
	dialogBodyStyle    = lipgloss.NewStyle().Padding(0, 1)
	dialogMutedStyle   = lipgloss.NewStyle().
				Foreground(styles.Muted).
				Padding(0, 1)

	dialogActiveStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#FFFFFF")).
				Background(styles.Primary).
				Bold(true).
				Padding(0, 2)

	dialogInactiveStyle = lipgloss.NewStyle().
				Foreground(styles.Muted).
				Padding(0, 2)

	dialogBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(styles.Warning).
			Padding(1, 2).
			Width(64)

	dialogErrorBoxStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(styles.Error).
				Padding(1, 2).
				Width(64)
)

const maxDependencyListLines = 6

func renderDependencyUpdateDialog(yesSelected bool, updatable []utils.DependencyUpdateEntry) string {
	yesBtn, noBtn := dialogInactiveStyle, dialogInactiveStyle
	if yesSelected {
		yesBtn = dialogActiveStyle
	} else {
		noBtn = dialogActiveStyle
	}

	buttons := lipgloss.JoinHorizontal(lipgloss.Center,
		yesBtn.Render("Yes"),
		"  ",
		noBtn.Render("No"),
	)

	lines := make([]string, 0, 6+len(updatable))
	lines = append(lines, dialogTitleStyle.Render(dialogWarningStyle.Render("⚠ Warning")))
	lines = append(lines, "")
	lines = append(lines, dialogBodyStyle.Render(fmt.Sprintf(
		"%d direct %s will be updated:",
		len(updatable),
		pluralize(len(updatable), "dependency", "dependencies"),
	)))

	visible := updatable
	extra := 0
	if len(visible) > maxDependencyListLines {
		extra = len(visible) - maxDependencyListLines
		visible = visible[:maxDependencyListLines]
	}
	for _, e := range visible {
		lines = append(lines, dialogBodyStyle.Render(fmt.Sprintf(
			"  %s: %s -> %s", e.Path, e.OldVersion, e.NewVersion,
		)))
	}
	if extra > 0 {
		lines = append(lines, dialogBodyStyle.Render(
			fmt.Sprintf("  …and %d more", extra),
		))
	}
	lines = append(lines, "")
	lines = append(lines, dialogBodyStyle.Render("go.mod and go.sum will be modified."))
	lines = append(lines, dialogBodyStyle.Render("A snapshot is taken before the update so changes can be rolled back."))
	lines = append(lines, "")
	lines = append(lines, buttons)

	return dialogBoxStyle.Render(lipgloss.JoinVertical(lipgloss.Left, lines...))
}

func renderDependencyChecksDialog(yesSelected bool) string {
	yesBtn, noBtn := dialogInactiveStyle, dialogInactiveStyle
	if yesSelected {
		yesBtn = dialogActiveStyle
	} else {
		noBtn = dialogActiveStyle
	}

	buttons := lipgloss.JoinHorizontal(lipgloss.Center,
		yesBtn.Render("Yes"),
		"  ",
		noBtn.Render("No"),
	)

	lines := []string{
		dialogTitleStyle.Render(styles.StatusInfoStyle.Render("✓ Run checks?")),
		"",
		dialogBodyStyle.Render("After the update the following will be executed:"),
		dialogBodyStyle.Render("  • go test ./..."),
		dialogBodyStyle.Render("  • go vet ./..."),
		"",
		dialogMutedStyle.Render("If a check fails you will be offered to roll back the dependencies."),
		"",
		buttons,
	}

	return dialogBoxStyle.Render(lipgloss.JoinVertical(lipgloss.Left, lines...))
}

func renderDependencyRollbackDialog(yesSelected bool, result *utils.DependencyCheckResultMsg) string {
	yesBtn, noBtn := dialogInactiveStyle, dialogInactiveStyle
	if yesSelected {
		yesBtn = dialogActiveStyle
	} else {
		noBtn = dialogActiveStyle
	}

	buttons := lipgloss.JoinHorizontal(lipgloss.Center,
		yesBtn.Render("Roll back"),
		"  ",
		noBtn.Render("Keep"),
	)

	lines := []string{
		dialogTitleStyle.Render(dialogWarningStyle.Render("⚠ Checks failed")),
		"",
	}
	if result != nil {
		lines = append(lines, dialogBodyStyle.Render(fmt.Sprintf("Command: %s", result.Command)))
		if result.Output != "" {
			for _, l := range strings.Split(result.Output, "\n") {
				lines = append(lines, dialogMutedStyle.Render(l))
			}
		}
		lines = append(lines, "")
	}
	lines = append(lines, dialogBodyStyle.Render("Roll back the dependencies to their pre-update state?"))
	lines = append(lines, "")
	lines = append(lines, buttons)

	return dialogErrorBoxStyle.Render(lipgloss.JoinVertical(lipgloss.Left, lines...))
}

func overlayDialog(background, dialog string, width, height int) string {
	if width <= 0 {
		width = 80
	}
	if height <= 0 {
		height = 24
	}
	_ = width

	// The dialog already has its own border/padding rendered by
	// dialogBoxStyle; we must not pad it out to a full-height canvas,
	// otherwise every background line outside the modal gets overwritten
	// with whitespace (the bug behind CleanShot 2026-06-27 at 17.54.47@2x.png
	// where the deps table was shredded by the confirm dialog).
	dialogLines := strings.Split(strings.TrimRight(dialog, "\n"), "\n")
	bgLines := strings.Split(background, "\n")
	if len(bgLines) == 0 {
		return background
	}

	if height > len(bgLines) {
		height = len(bgLines)
	}

	// Center the actual dialog lines vertically on the background.
	startRow := 0
	if len(bgLines) > len(dialogLines) {
		startRow = (len(bgLines) - len(dialogLines)) / 2
	}
	if startRow+len(dialogLines) > height {
		// Keep the dialog inside the visible viewport.
		startRow = height - len(dialogLines)
		if startRow < 0 {
			startRow = 0
		}
	}
	endRow := startRow + len(dialogLines)
	if endRow > len(bgLines) {
		endRow = len(bgLines)
	}
	dialogLines = dialogLines[:endRow-startRow]

	for i, dline := range dialogLines {
		row := startRow + i
		bgLine := bgLines[row]
		bgW := ansi.StringWidth(bgLine)
		dW := ansi.StringWidth(dline)
		col := 0
		if bgW > dW {
			col = (bgW - dW) / 2
		}
		bgLines[row] = spliceCentered(bgLine, dline, col)
	}

	return strings.Join(bgLines, "\n")
}

func spliceCentered(bg, overlay string, col int) string {
	if col < 0 {
		col = 0
	}
	// col is a column index measured in visible cells (the value the caller
	// got from ansi.StringWidth / lipgloss.Width). Slicing by []rune would
	// (a) drop a wide rune that straddles the cut point and (b) chop ANSI
	// escape sequences in half, which corrupts the surrounding styled
	// table output. Use ANSI-aware cuts instead.
	bgW := ansi.StringWidth(bg)
	if col > bgW {
		col = bgW
	}
	ovW := ansi.StringWidth(overlay)

	prefix := ansi.Cut(bg, 0, col)
	suffix := ""
	if col+ovW < bgW {
		suffix = ansi.Cut(bg, col+ovW, bgW)
	}
	return prefix + overlay + suffix
}
