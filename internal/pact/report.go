// Package pact checks an application against its Pact contract: what the
// contract says, whether the environment would make the application refuse
// to start, how Cauthy sees it, and what the deployed application serves.
package pact

import (
	"fmt"
	"strings"
	"unicode"
)

// The statuses a line carries, as the conformance runner's.
const (
	Pass     = "pass"
	Fail     = "fail"
	Skip     = "skip"     // asked for, and could not run: the run is incomplete
	NA       = "n/a"      // does not apply to this application
	Advisory = "advisory" // worth fixing; does not fail the run
)

type Line struct {
	ID     string
	Status string
	Detail string
}

type Report struct{ Lines []Line }

func (r *Report) Add(id, status, format string, a ...any) {
	r.Lines = append(r.Lines, Line{ID: id, Status: status, Detail: fmt.Sprintf(format, a...)})
}

// Clean drops the control characters (and bidirectional overrides) from text
// that came from a graph, a server or the environment, so it cannot move the
// cursor, retitle the terminal or reorder what a person reads.
func Clean(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.Is(unicode.Bidi_Control, r) {
			return -1
		}
		return r
	}, s)
}

// CleanLines is Clean for text of several lines: the newlines stay.
func CleanLines(s string) string {
	lines := strings.Split(s, "\n")
	for i := range lines {
		lines[i] = Clean(lines[i])
	}
	return strings.Join(lines, "\n")
}

// Text is one line per check, then the counts, as the runner prints them.
func (r *Report) Text() string {
	var b strings.Builder
	counts := map[string]int{}
	for _, l := range r.Lines {
		counts[l.Status]++
		fmt.Fprintf(&b, "%-8s %s  %s\n", strings.ToUpper(l.Status), Clean(l.ID), Clean(l.Detail))
	}
	fmt.Fprintf(&b, "\n%d passed, %d failed, %d skipped, %d advisory\n", counts[Pass], counts[Fail], counts[Skip], counts[Advisory])
	if counts[Skip] > 0 && counts[Fail] == 0 {
		fmt.Fprintf(&b, "INCOMPLETE: %d skipped\n", counts[Skip])
	}
	return b.String()
}

// ExitCode is 1 when a line failed, 3 when one was skipped, else 0.
func (r *Report) ExitCode() int {
	code := 0
	for _, l := range r.Lines {
		switch l.Status {
		case Fail:
			return 1
		case Skip:
			code = 3
		}
	}
	return code
}
