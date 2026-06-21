package styles

import (
	"regexp"
	"strings"
	"testing"
)

var ansiPattern = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func plainText(s string) string {
	return ansiPattern.ReplaceAllString(s, "")
}

func TestItemTitleRendersStatusBadgesWithoutLegacyParentheses(t *testing.T) {
	item := Item{
		Name:      "1.24.4",
		Installed: true,
		Active:    true,
	}

	title := plainText(item.Title())

	for _, want := range []string{"1.24.4", "active", "installed"} {
		if !strings.Contains(title, want) {
			t.Fatalf("expected title %q to contain %q", title, want)
		}
	}

	if strings.Contains(title, "(active)") || strings.Contains(title, "(installed)") {
		t.Fatalf("expected modern badge text without legacy parentheses, got %q", title)
	}
}

func TestItemDescriptionReturnsSecondaryText(t *testing.T) {
	item := Item{DescriptionText: "go1.24.4.darwin-arm64.tar.gz"}

	if got := item.Description(); got != "go1.24.4.darwin-arm64.tar.gz" {
		t.Fatalf("expected description to preserve secondary text, got %q", got)
	}
}
