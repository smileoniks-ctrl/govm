// Package version defines the canonical grammar for Go versions accepted by govm.
package version

import (
	"fmt"
	"regexp"
	"strings"
)

var canonicalPattern = regexp.MustCompile(`^[0-9]+\.[0-9]+(?:(?:\.[0-9]+)|(?:beta|rc)[0-9]+)?$`)

// InvalidError reports a version string that is not in govm's canonical grammar.
type InvalidError struct {
	Input string
}

func (e *InvalidError) Error() string {
	return fmt.Sprintf("invalid Go version %q", e.Input)
}

// Normalize removes one optional "go" prefix and validates the resulting
// canonical bare version.
func Normalize(input string) (string, error) {
	canonical := strings.TrimPrefix(input, "go")
	if err := Validate(canonical); err != nil {
		return "", &InvalidError{Input: input}
	}
	return canonical, nil
}

// Validate reports whether value is a canonical bare Go version.
func Validate(value string) error {
	if !canonicalPattern.MatchString(value) {
		return &InvalidError{Input: value}
	}
	return nil
}
