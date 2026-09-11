package styles

import (
	"fmt"
	"strings"
)

// Item is a single row in the Available Versions list. It carries the
// data bubbles/list needs (Name as FilterValue, DescriptionText as
// Description) plus a pre-rendered title string. Pre-rendering the
// title when the version catalog builds its projection keeps
// lipgloss.Style.Render calls out of the per-frame list.View hot path,
// and lets Item stay a pure data carrier with no theme dependency.
type Item struct {
	Name            string
	DescriptionText string
	RenderedTitle   string
	// RenderedBadges holds the pre-rendered status badges alone (no
	// version name). The Available list delegate appends it to a
	// freshly highlighted Name while a filter is active, because match
	// highlighting must never run over RenderedTitle: its ANSI codes
	// would be split mid-sequence.
	RenderedBadges string
}

// Title returns the pre-rendered title. bubbles/list.DefaultDelegate
// calls this method on every visible item on every frame, so it must
// stay a field access — never re-introduce a lipgloss.Render here.
func (i Item) Title() string { return i.RenderedTitle }

func (i Item) FilterValue() string { return i.Name }

func (i Item) Description() string {
	if i.DescriptionText == "" {
		return fmt.Sprintf("go%s", i.Name)
	}

	desc := i.DescriptionText
	if len(desc) > 50 {
		return desc[:47] + "..."
	}
	return desc
}

// RenderItemTitle produces the styled title string for a list row:
// version name, plus "active" and "installed" status badges when
// applicable. It is the single owner of the title format and is
// called once per item per list rebuild, not per frame.
func RenderItemTitle(t Theme, name string, installed, active bool) string {
	title := t.ItemVersionStyle.Render(name)
	if badges := RenderItemBadges(t, installed, active); badges != "" {
		title += " " + badges
	}
	return title
}

// RenderItemBadges produces the styled "active" / "installed" badges
// for a list row, space-separated, or "" when neither applies. It is
// the badge half of RenderItemTitle so the list delegate can rebuild a
// title around a highlighted version name without re-deriving the
// badge format.
func RenderItemBadges(t Theme, installed, active bool) string {
	var parts []string
	if active {
		parts = append(parts, t.ActiveBadgeStyle.Render("active"))
	}
	if installed {
		parts = append(parts, t.InstalledBadgeStyle.Render("installed"))
	}
	return strings.Join(parts, " ")
}
