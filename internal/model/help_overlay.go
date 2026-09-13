package model

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/smileoniks-ctrl/govm/internal/styles"
)

// helpOverlaySections resolves which registry sections the Help
// overlay shows: the bindings of the Input context beneath the overlay
// (open dialog, prune confirmation, delete confirmation, or tab)
// followed by the matching global bindings. Text-entry contexts never
// appear here: the overlay cannot open while an input has focus.
func helpOverlaySections(m Model) []helpSection {
	return contextKeyBindings(m, m.inputContextBeneathHelp())
}

// renderHelpOverlay builds the themed Help overlay box: a title, then
// one block per section with keys aligned in a single column. There
// is no scrolling: when the viewport is too short the bottom of the
// content is truncated, because the dialog box border and padding
// cost four rows of the viewport height.
func renderHelpOverlay(t styles.Theme, m Model, viewport viewportSize) string {
	sections := helpOverlaySections(m)

	lines := []string{t.DialogTitleStyle.Render("Keyboard Shortcuts")}
	for _, section := range sections {
		keyWidth := 0
		for _, binding := range section.bindings {
			if w := lipgloss.Width(binding.keys); w > keyWidth {
				keyWidth = w
			}
		}
		lines = append(lines, "")
		lines = append(lines, t.DialogMutedStyle.Render(strings.ToUpper(section.title)))
		for _, binding := range section.bindings {
			key := binding.keys + strings.Repeat(" ", keyWidth-lipgloss.Width(binding.keys))
			lines = append(lines, t.DialogBodyStyle.Render(fmt.Sprintf(
				"%s  %s",
				t.HelpKeyStyle.Render(key),
				t.HelpTextStyle.Render(binding.desc),
			)))
		}
	}

	// dialog border (2 rows) + vertical padding (2 rows)
	maxLines := viewport.Height - 4
	if maxLines < 1 {
		maxLines = 1
	}
	if len(lines) > maxLines {
		lines = lines[:maxLines]
	}
	return renderDialog(t, strings.Join(lines, "\n"), false, viewport)
}
