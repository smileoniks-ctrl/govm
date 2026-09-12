package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/smileoniks-ctrl/govm/internal/doctor"
)

// Doctor runs the read-only diagnostics and renders the Report in the
// plain-text layout users paste into bug reports. It returns false
// when the arguments are invalid or any Check failed; warn verdicts
// never change the result.
func (a *App) Doctor(args ...string) bool {
	offline, err := parseDoctorArgs(args)
	if err != nil {
		fmt.Fprintf(a.out, "Error: %v\n", err)
		fmt.Fprintln(a.out, doctorUsage)
		return false
	}
	if a.operations.Doctor == nil {
		fmt.Fprintln(a.out, "Error: doctor is not configured")
		return false
	}
	report := a.operations.Doctor(context.Background(), offline)
	renderDoctorReport(a.out, report)
	return !report.Failed()
}

const doctorUsage = "usage: govm doctor [--offline]"

func parseDoctorArgs(args []string) (offline bool, err error) {
	for _, arg := range args {
		switch arg {
		case "--offline":
			offline = true
		case "--help", "-h":
			return false, errors.New("help requested")
		default:
			return false, fmt.Errorf("unknown doctor option %q", arg)
		}
	}
	return offline, nil
}

// verdictColumn is the width of the "[verdict]" column; hint lines are
// indented past it so they sit under the Check detail.
const verdictColumn = len("[warn]")

// renderDoctorReport prints the environment header, one padded
// verdict line per Check with an indented hint for warn/fail, and the
// summary line. No colours: the output is meant to be copied as-is.
func renderDoctorReport(out io.Writer, report doctor.Report) {
	root := report.Root
	if root == "" {
		root = "unknown"
	}
	fmt.Fprintf(out, "govm doctor  (govm %s, %s/%s, root %s)\n\n",
		report.GovmVersion, report.OS, report.Arch, root)
	hintIndent := strings.Repeat(" ", verdictColumn+1)
	for _, c := range report.Checks {
		fmt.Fprintf(out, "%-*s %s\n", verdictColumn, "["+string(c.Verdict)+"]", c.Detail)
		if c.Verdict != doctor.VerdictOK && c.Hint != "" {
			fmt.Fprintf(out, "%shint: %s\n", hintIndent, c.Hint)
		}
	}
	fails, warns := report.Counts()
	if fails == 0 && warns == 0 {
		fmt.Fprintln(out, "\nall checks passed")
		return
	}
	fmt.Fprintf(out, "\n%d fail, %d warn\n", fails, warns)
}
