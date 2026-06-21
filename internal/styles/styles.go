package styles

import (
	"fmt"
	"strings"
)

type Item struct {
	Name            string
	DescriptionText string
	Installed       bool
	Active          bool
}

func (i Item) Title() string {
	parts := []string{ItemVersionStyle.Render(i.Name)}
	if i.Active {
		parts = append(parts, ActiveBadgeStyle.Render("active"))
	}
	if i.Installed {
		parts = append(parts, InstalledBadgeStyle.Render("installed"))
	}
	return strings.Join(parts, " ")
}

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
