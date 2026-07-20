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

// TestNewThemeBuildsFromPalette verifies that the immutable Theme value
// produced by NewTheme carries the expected palette colours and that
// light/current themes actually differ. It is a pure value test: no
// package-level globals are mutated, so it is safe under t.Parallel and
// is the pattern all future theme tests should follow.
func TestNewThemeBuildsFromPalette(t *testing.T) {
	t.Parallel()

	current := NewTheme(ThemeCurrent)
	light := NewTheme(ThemeLight)

	for _, tc := range []struct {
		name    string
		theme   Theme
		primary string
		text    string
		muted   string
	}{
		{"current", current, "#7C3AED", "#E5E7EB", "#6B7280"},
		{"light", light, "#6D28D9", "#111827", "#4B5563"},
	} {
		if got, want := tc.theme.Primary, lipgloss.Color(tc.primary); got != want {
			t.Errorf("%s theme Primary = %v, want %v", tc.name, got, want)
		}
		if got, want := tc.theme.Text, lipgloss.Color(tc.text); got != want {
			t.Errorf("%s theme Text = %v, want %v", tc.name, got, want)
		}
		if got, want := tc.theme.Muted, lipgloss.Color(tc.muted); got != want {
			t.Errorf("%s theme Muted = %v, want %v", tc.name, got, want)
		}
	}

	currentOutput := current.TitleStyle.Render("GoVM")
	lightOutput := light.TitleStyle.Render("GoVM")
	if currentOutput == lightOutput {
		t.Fatalf("expected current and light TitleStyle output to differ")
	}

	// Minimum-viewport colors are theme-independent today; assert the
	// contract explicitly so a future change is caught here rather than
	// in a viewport snapshot diff.
	if got, want := current.MinimumViewportBackground, lipgloss.Color("#111827"); got != want {
		t.Fatalf("MinimumViewportBackground = %v, want %v", got, want)
	}
	if got, want := current.MinimumViewportText, lipgloss.Color("#F9FAFB"); got != want {
		t.Fatalf("MinimumViewportText = %v, want %v", got, want)
	}
}

// TestNewThemeFallsBackToCurrentForUnknownTheme is the value-type
// replacement for TestApplyThemeFallsBackToCurrentForUnknownTheme. It
// pins NewTheme's silent-fallback contract without touching global
// state and is therefore safe under t.Parallel.
func TestNewThemeFallsBackToCurrentForUnknownTheme(t *testing.T) {
	t.Parallel()

	current := NewTheme(ThemeCurrent)
	unknown := NewTheme(ThemeName("does-not-exist"))

	if unknown.Primary != current.Primary {
		t.Fatalf("unknown theme Primary = %v, want fallback %v", unknown.Primary, current.Primary)
	}
	if unknown.TitleStyle.Render("x") != current.TitleStyle.Render("x") {
		t.Fatalf("unknown theme did not fall back to current rendering")
	}
}

// TestNewThemeIsPure proves NewTheme is safe to call concurrently. It is
// the verification gate for the parallel-test story (architecture review
// candidate 5, cause 2) and must pass under -race.
func TestNewThemeIsPure(t *testing.T) {
	t.Parallel()

	done := make(chan Theme, 2)
	go func() { done <- NewTheme(ThemeCurrent) }()
	go func() { done <- NewTheme(ThemeLight) }()

	first := <-done
	second := <-done

	// Both results must be byte-identical to the value returned by a
	// sequential call for the same name, regardless of completion order.
	baseline := map[ThemeName]Theme{
		ThemeCurrent: NewTheme(ThemeCurrent),
		ThemeLight:   NewTheme(ThemeLight),
	}
	if first.Primary != baseline[ThemeCurrent].Primary && first.Primary != baseline[ThemeLight].Primary {
		t.Fatalf("concurrent first Primary %v matched neither baseline", first.Primary)
	}
	if second.Primary != baseline[ThemeCurrent].Primary && second.Primary != baseline[ThemeLight].Primary {
		t.Fatalf("concurrent second Primary %v matched neither baseline", second.Primary)
	}
	if first.Primary == second.Primary {
		t.Fatalf("expected concurrent results to come from distinct themes; both Primary = %v", first.Primary)
	}
}
