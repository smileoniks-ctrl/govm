package main

import (
	"io"
	"os"
	"strings"
	"testing"

	"github.com/smileoniks-ctrl/govm/internal/utils"
)

func TestPrintUsageShowsVersion(t *testing.T) {
	prev := utils.Version
	utils.Version = "v9.9.9-test"
	defer func() { utils.Version = prev }()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	orig := os.Stdout
	os.Stdout = w

	printUsage()

	w.Close()
	os.Stdout = orig

	data, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	output := string(data)

	if !strings.Contains(output, "v9.9.9-test") {
		t.Fatalf("expected help output to contain version %q, got:\n%s", "v9.9.9-test", output)
	}

	if !strings.Contains(output, "GoVM") {
		t.Fatal("expected help output to mention GoVM")
	}
}
