package styles

import (
	"regexp"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
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

func TestApplyThemeFallsBackToCurrentForUnknownTheme(t *testing.T) {
	t.Cleanup(func() {
		ApplyTheme(ThemeCurrent)
	})

	if got := ApplyTheme(ThemeName("unknown")); got != ThemeCurrent {
		t.Fatalf("expected unknown theme to fall back to %q, got %q", ThemeCurrent, got)
	}

	if got := CurrentTheme(); got != ThemeCurrent {
		t.Fatalf("expected current theme to be %q, got %q", ThemeCurrent, got)
	}
}

func TestApplyThemeCurrentRestoresInstalledBadgeForeground(t *testing.T) {
	t.Cleanup(func() {
		ApplyTheme(ThemeCurrent)
	})

	ApplyTheme(ThemeCurrent)

	if got, want := InstalledBadgeStyle.GetForeground(), lipgloss.Color("#F8FAFC"); got != want {
		t.Fatalf("expected installed badge foreground to be %q, got %q", want, got)
	}
}

func TestApplyThemeLightChangesAndCurrentRestoresRenderedOutput(t *testing.T) {
	t.Cleanup(func() {
		ApplyTheme(ThemeCurrent)
	})

	ApplyTheme(ThemeCurrent)
	currentOutput := TitleStyle.Render("GoVM")

	if got := ApplyTheme(ThemeLight); got != ThemeLight {
		t.Fatalf("expected light theme to apply as %q, got %q", ThemeLight, got)
	}

	lightOutput := TitleStyle.Render("GoVM")
	if lightOutput == currentOutput {
		t.Fatalf("expected light theme output to differ from current output")
	}

	if got := ApplyTheme(ThemeCurrent); got != ThemeCurrent {
		t.Fatalf("expected current theme to apply as %q, got %q", ThemeCurrent, got)
	}

	restoredOutput := TitleStyle.Render("GoVM")
	if restoredOutput != currentOutput {
		t.Fatalf("expected current theme to restore output %q, got %q", currentOutput, restoredOutput)
	}
}
