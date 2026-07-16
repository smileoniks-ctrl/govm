package model

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/smileoniks-ctrl/govm/internal/styles"
	"github.com/smileoniks-ctrl/govm/internal/utils"
)

// Static styles for modal dialogs. They are recomputed only when the user
// changes the active theme.
var (
	dialogTitleStyle    lipgloss.Style
	dialogWarningStyle  lipgloss.Style
	dialogBodyStyle     lipgloss.Style
	dialogMutedStyle    lipgloss.Style
	dialogActiveStyle   lipgloss.Style
	dialogInactiveStyle lipgloss.Style
	dialogBoxStyle      lipgloss.Style
	dialogErrorBoxStyle lipgloss.Style
)

func init() {
	rebuildDialogStyles()
}

func rebuildDialogStyles() {
	dialogTitleStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(styles.Warning).
		Padding(0, 1)

	dialogWarningStyle = styles.StatusWarningStyle
	dialogBodyStyle = lipgloss.NewStyle().Padding(0, 1)
	dialogMutedStyle = lipgloss.NewStyle().
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
}

const maxDependencyListLines = 6

type viewportSize struct {
	Width  int
	Height int
}

func dialogWidth(viewport viewportSize) int {
	width := viewport.Width
	if width < 1 {
		width = 64
	}
	return min(64, width)
}

func renderDialog(content string, errorStyle bool, viewport viewportSize) string {
	width := dialogWidth(viewport)
	contentWidth := max(1, width-6)
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		lines[i] = ansi.Cut(line, 0, contentWidth)
	}

	style := dialogBoxStyle
	if errorStyle {
		style = dialogErrorBoxStyle
	}
	return style.Width(width).Render(strings.Join(lines, "\n"))
}

func renderDependencyUpdateDialog(yesSelected bool, updatable []utils.DependencyUpdateEntry, viewport viewportSize) string {
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
		utils.Pluralize(len(updatable), "dependency", "dependencies"),
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

	return renderDialog(lipgloss.JoinVertical(lipgloss.Left, lines...), false, viewport)
}

func renderDependencyChecksDialog(yesSelected bool, viewport viewportSize) string {
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

	return renderDialog(lipgloss.JoinVertical(lipgloss.Left, lines...), false, viewport)
}

func renderDependencyRollbackDialog(yesSelected bool, result *utils.DependencyCheckResultMsg, viewport viewportSize) string {
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
			output := strings.Split(result.Output, "\n")
			visible := output
			if len(visible) > maxDependencyListLines {
				visible = visible[:maxDependencyListLines]
			}
			for _, l := range visible {
				lines = append(lines, dialogMutedStyle.Render(l))
			}
			if extra := len(output) - len(visible); extra > 0 {
				lines = append(lines, dialogMutedStyle.Render(fmt.Sprintf("…and %d more", extra)))
			}
		}
		lines = append(lines, "")
	}
	lines = append(lines, dialogBodyStyle.Render("Roll back the dependencies to their pre-update state?"))
	lines = append(lines, "")
	lines = append(lines, buttons)

	return renderDialog(lipgloss.JoinVertical(lipgloss.Left, lines...), true, viewport)
}

func renderDependencyRestoreDialog(yesSelected bool, backups []utils.DependencyBackupInfo, cursor int, viewport viewportSize) string {
	yesBtn, noBtn := dialogInactiveStyle, dialogInactiveStyle
	if yesSelected {
		yesBtn = dialogActiveStyle
	} else {
		noBtn = dialogActiveStyle
	}

	buttons := lipgloss.JoinHorizontal(lipgloss.Center,
		yesBtn.Render("Restore"),
		"  ",
		noBtn.Render("Cancel"),
	)

	lines := []string{
		dialogTitleStyle.Render(dialogWarningStyle.Render("Dependency backups")),
		"",
		dialogBodyStyle.Render("Choose a saved dependency backup:"),
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
		lines = append(lines, dialogBodyStyle.Render(fmt.Sprintf(
			"%s%s  %s  %d update(s)",
			prefix,
			b.Name,
			b.Kind,
			b.Updated,
		)))
	}
	if len(backups) > end {
		lines = append(lines, dialogBodyStyle.Render(fmt.Sprintf("  …and %d more", len(backups)-end)))
	}
	lines = append(lines, "")
	lines = append(lines, dialogMutedStyle.Render("Current go.mod and go.sum will be saved before restore."))
	lines = append(lines, "")
	lines = append(lines, buttons)

	return renderDialog(lipgloss.JoinVertical(lipgloss.Left, lines...), false, viewport)
}

func overlayDialog(background, dialog string, viewport viewportSize) string {
	width, height := viewport.Width, viewport.Height
	if width < 1 {
		width = 80
	}
	if height < 1 {
		height = 24
	}

	dialogLines := strings.Split(strings.TrimRight(dialog, "\n"), "\n")
	bgLines := strings.Split(background, "\n")
	if len(bgLines) > height {
		bgLines = bgLines[:height]
	}
	blankRow := strings.Repeat(" ", width)
	for len(bgLines) < height {
		bgLines = append(bgLines, blankRow)
	}

	startRow := 0
	if height > len(dialogLines) {
		startRow = (height - len(dialogLines)) / 2
	}
	if startRow+len(dialogLines) > height {
		startRow = height - len(dialogLines)
		if startRow < 0 {
			startRow = 0
		}
	}
	endRow := startRow + len(dialogLines)
	if endRow > height {
		endRow = height
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
		bgLines[row] = spliceCentered(bgLine, dline, col, bgW, dW)
	}

	return strings.Join(bgLines, "\n")
}

func spliceCentered(bg, overlay string, col, bgW, overlayW int) string {
	if col < 0 {
		col = 0
	}
	// col is a column index measured in visible cells (the value the caller
	// got from ansi.StringWidth / lipgloss.Width). Slicing by []rune would
	// (a) drop a wide rune that straddles the cut point and (b) chop ANSI
	// escape sequences in half, which corrupts the surrounding styled
	// table output. Use ANSI-aware cuts instead.
	if col > bgW {
		col = bgW
	}

	prefix := ansi.Cut(bg, 0, col)
	suffix := ""
	if col+overlayW < bgW {
		suffix = ansi.Cut(bg, col+overlayW, bgW)
	}
	return prefix + overlay + suffix
}
