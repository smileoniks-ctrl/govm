package version

import (
	"errors"
	"testing"
)

func TestValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		value string
		valid bool
	}{
		{name: "major minor", value: "1.26", valid: true},
		{name: "patch", value: "1.26.1", valid: true},
		{name: "beta", value: "1.27beta2", valid: true},
		{name: "release candidate", value: "1.27rc1", valid: true},
		{name: "zero components", value: "0.0.0", valid: true},
		{name: "empty", value: "", valid: false},
		{name: "go prefix", value: "go1.26.1", valid: false},
		{name: "missing minor", value: "1", valid: false},
		{name: "missing prerelease number", value: "1.27rc", valid: false},
		{name: "prerelease after patch", value: "1.27.0rc1", valid: false},
		{name: "uppercase prerelease", value: "1.27RC1", valid: false},
		{name: "leading whitespace", value: " 1.26.1", valid: false},
		{name: "trailing whitespace", value: "1.26.1 ", valid: false},
		{name: "metadata", value: "1.26.1+build", valid: false},
		{name: "path traversal", value: "../1.26.1", valid: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := Validate(tt.value)
			if tt.valid && err != nil {
				t.Fatalf("Validate(%q) error = %v", tt.value, err)
			}
			if !tt.valid {
				var invalid *InvalidError
				if !errors.As(err, &invalid) {
					t.Fatalf("Validate(%q) error = %v, want *InvalidError", tt.value, err)
				}
			}
		})
	}
}

func TestNormalize(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "bare", input: "1.26.1", want: "1.26.1"},
		{name: "go prefix", input: "go1.26.1", want: "1.26.1"},
		{name: "beta prefix", input: "go1.27beta1", want: "1.27beta1"},
		{name: "double prefix", input: "gogo1.26.1", wantErr: true},
		{name: "uppercase prefix", input: "Go1.26.1", wantErr: true},
		{name: "prefix only", input: "go", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := Normalize(tt.input)
			if tt.wantErr {
				var invalid *InvalidError
				if !errors.As(err, &invalid) {
					t.Fatalf("Normalize(%q) error = %v, want *InvalidError", tt.input, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Normalize(%q) error = %v", tt.input, err)
			}
			if got != tt.want {
				t.Fatalf("Normalize(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
