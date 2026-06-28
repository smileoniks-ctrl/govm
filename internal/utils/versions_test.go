package utils

import (
	"reflect"
	"testing"
)

func TestCompareGoVersions(t *testing.T) {
	cases := []struct {
		name     string
		a, b     string
		expected int
	}{
		{"equal", "1.22.0", "1.22.0", 0},
		{"major-greater", "2.0.0", "1.99.99", 1},
		{"major-lesser", "1.99.99", "2.0.0", -1},
		{"minor-greater", "1.23.0", "1.22.99", 1},
		{"minor-lesser", "1.22.99", "1.23.0", -1},
		{"patch-greater", "1.22.10", "1.22.9", 1},
		{"patch-lesser", "1.22.9", "1.22.10", -1},
		{"a-shorter-is-lesser-when-prefix-equal", "1.21", "1.21.1", -1},
		{"a-longer-is-greater-when-prefix-equal", "1.21.1", "1.21", 1},
		{"shorter-vs-larger-minor", "1.21", "1.22", -1},
		{"extra-segment-treated-as-zero", "1.22.0.0", "1.22", 0},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := CompareGoVersions(tc.a, tc.b)
			if sign(got) != tc.expected {
				t.Fatalf("CompareGoVersions(%q, %q) = %d, want %d", tc.a, tc.b, got, tc.expected)
			}
		})
	}
}

func TestSortGoVersionsDesc(t *testing.T) {
	in := []string{"1.21.5", "1.22.0", "1.21.10", "1.22.1", "1.20.0", "1.22"}
	want := []string{"1.22.1", "1.22.0", "1.22", "1.21.10", "1.21.5", "1.20.0"}

	SortGoVersionsDesc(in)

	if !reflect.DeepEqual(in, want) {
		t.Fatalf("SortGoVersionsDesc mismatch:\n got: %v\nwant: %v", in, want)
	}
}

func TestFindLatestGoVersion(t *testing.T) {
	versions := []string{"1.21.5", "1.22.0", "1.21.10", "1.22.1", "1.20.0", "1.22"}

	t.Run("exact match wins over prefix", func(t *testing.T) {
		got, ok := FindLatestGoVersion(versions, "1.22.0")
		if !ok || got != "1.22.0" {
			t.Fatalf("expected exact match 1.22.0, got %q (ok=%v)", got, ok)
		}
	})

	t.Run("prefix returns latest minor", func(t *testing.T) {
		got, ok := FindLatestGoVersion(versions, "1.21")
		if !ok || got != "1.21.10" {
			t.Fatalf("expected latest 1.21.x = 1.21.10, got %q (ok=%v)", got, ok)
		}
	})

	t.Run("major prefix returns latest overall major", func(t *testing.T) {
		got, ok := FindLatestGoVersion(versions, "1")
		if !ok || got != "1.22.1" {
			t.Fatalf("expected latest 1.x = 1.22.1, got %q (ok=%v)", got, ok)
		}
	})

	t.Run("no match returns false", func(t *testing.T) {
		got, ok := FindLatestGoVersion(versions, "2")
		if ok {
			t.Fatalf("expected no match for major 2, got %q", got)
		}
	})
}

func TestNormalizeGoVersionQuery(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"1.22", "1.22"},
		{"go1.22", "1.22"},
		{"v1.22.1", "1.22.1"},
		{"  go1.22.0  ", "1.22.0"},
		{"", ""},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			got := NormalizeGoVersionQuery(tc.in)
			if got != tc.want {
				t.Fatalf("NormalizeGoVersionQuery(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func sign(v int) int {
	switch {
	case v < 0:
		return -1
	case v > 0:
		return 1
	default:
		return 0
	}
}
