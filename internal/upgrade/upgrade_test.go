package upgrade

import (
	"context"
	"errors"
	"testing"
)

func TestReleaseBuild(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in   string
		want string
		ok   bool
	}{
		{in: "0.2.4", want: "v0.2.4", ok: true},
		{in: "v0.2.4", want: "v0.2.4", ok: true},
		{in: " v0.2.4 ", want: "v0.2.4", ok: true},
		{in: "v0.3.0-rc1", want: "v0.3.0-rc1", ok: true},
		{in: "v0.2.5-0.20260912123456-abcdef123456", want: "v0.2.5-0.20260912123456-abcdef123456", ok: true},
		{in: "dev", ok: false},
		{in: "(devel)", ok: false},
		{in: "abcdef12 (2026-09-12)", ok: false},
		{in: "", ok: false},
		{in: "v1", want: "v1", ok: true},
	}
	for _, tt := range tests {
		got, ok := ReleaseBuild(tt.in)
		if ok != tt.ok || got != tt.want {
			t.Errorf("ReleaseBuild(%q) = (%q, %v), want (%q, %v)", tt.in, got, ok, tt.want, tt.ok)
		}
	}
}

func TestNewer(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		current string
		latest  string
		want    string
		newer   bool
	}{
		{name: "goreleaser build behind tag", current: "0.2.4", latest: "v0.2.5", want: "v0.2.5", newer: true},
		{name: "equal", current: "0.2.4", latest: "v0.2.4", newer: false},
		{name: "ahead of latest", current: "v0.2.6", latest: "v0.2.5", newer: false},
		{name: "pseudo-version below next patch", current: "v0.2.5-0.20260912123456-abcdef123456", latest: "v0.2.5", want: "v0.2.5", newer: true},
		{name: "pseudo-version above previous patch", current: "v0.2.5-0.20260912123456-abcdef123456", latest: "v0.2.4", newer: false},
		{name: "release candidate above stable", current: "v0.3.0-rc1", latest: "v0.2.5", newer: false},
		{name: "release candidate below its final", current: "v0.3.0-rc1", latest: "v0.3.0", want: "v0.3.0", newer: true},
		{name: "dev build never newer", current: "dev", latest: "v9.9.9", newer: false},
		{name: "unparsable tag", current: "v0.2.4", latest: "latest", newer: false},
		{name: "major bump", current: "v0.9.9", latest: "v1.0.0", want: "v1.0.0", newer: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, newer := Newer(tt.current, tt.latest)
			if newer != tt.newer || got != tt.want {
				t.Fatalf("Newer(%q, %q) = (%q, %v), want (%q, %v)", tt.current, tt.latest, got, newer, tt.want, tt.newer)
			}
		})
	}
}

type fakeSource struct {
	tag   string
	err   error
	calls int
}

func (f *fakeSource) LatestRelease(context.Context) (string, error) {
	f.calls++
	return f.tag, f.err
}

func TestCheckerReportsNewerRelease(t *testing.T) {
	t.Parallel()
	source := &fakeSource{tag: "v0.2.5"}
	notice, ok, err := NewChecker(source, "0.2.4").Check(context.Background())
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if !ok || notice.Latest != "v0.2.5" {
		t.Fatalf("Check() = (%+v, %v), want (v0.2.5, true)", notice, ok)
	}
	if source.calls != 1 {
		t.Fatalf("source called %d times, want 1", source.calls)
	}
}

func TestCheckerUpToDate(t *testing.T) {
	t.Parallel()
	source := &fakeSource{tag: "v0.2.4"}
	notice, ok, err := NewChecker(source, "v0.2.4").Check(context.Background())
	if err != nil || ok || notice != (Notice{}) {
		t.Fatalf("Check() = (%+v, %v, %v), want (zero, false, nil)", notice, ok, err)
	}
}

func TestCheckerSkipsNonReleaseBuildWithoutConsultingSource(t *testing.T) {
	t.Parallel()
	for _, current := range []string{"dev", "abcdef12 (2026-09-12)", "(devel)", ""} {
		source := &fakeSource{tag: "v9.9.9"}
		notice, ok, err := NewChecker(source, current).Check(context.Background())
		if err != nil || ok || notice != (Notice{}) {
			t.Fatalf("Check() for %q = (%+v, %v, %v), want (zero, false, nil)", current, notice, ok, err)
		}
		if source.calls != 0 {
			t.Fatalf("source consulted for non-release build %q", current)
		}
	}
}

func TestCheckerPropagatesSourceError(t *testing.T) {
	t.Parallel()
	boom := errors.New("boom")
	source := &fakeSource{err: boom}
	_, ok, err := NewChecker(source, "v0.2.4").Check(context.Background())
	if ok {
		t.Fatal("Check() reported a notice despite source error")
	}
	if !errors.Is(err, boom) {
		t.Fatalf("Check() error = %v, want wrapping %v", err, boom)
	}
}

func TestCheckerWithoutSource(t *testing.T) {
	t.Parallel()
	if _, ok, err := NewChecker(nil, "v0.2.4").Check(context.Background()); ok || err == nil {
		t.Fatalf("Check() = (%v, %v), want (false, error)", ok, err)
	}
	var nilChecker *Checker
	if _, ok, err := nilChecker.Check(context.Background()); ok || err == nil {
		t.Fatalf("nil Check() = (%v, %v), want (false, error)", ok, err)
	}
}
