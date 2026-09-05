// Command check-conflict-markers fails when a file carries a version-control
// conflict marker, so that a marker cannot be committed and then live on in
// the tree unnoticed.
//
// It differs from the usual check in two ways. It knows the diff3 base marker,
// which the three-marker checks do not, and it runs at every commit rather
// than only while a merge is in progress — a marker that was already committed
// is exactly the one a merge-only check can no longer reach.
//
// It is designed to run as a pre-commit hook but works standalone:
//
//	check-conflict-markers file.go docs/spec.md
//	check-conflict-markers -root ./docs
//	check-conflict-markers -exclude docs/merging.md,testdata/
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/blairham/pre-commit-hooks/internal/conflictmarkers"
)

// Exit codes are API: 0 clean, 1 a marker was found, 2 the check itself
// failed. A human debugging needs the difference even though pre-commit
// treats both non-zero codes as failure.
const (
	exitOK        = 0
	exitViolation = 1
	exitFailure   = 2
)

func main() {
	opts := conflictmarkers.Default()
	var (
		exclude = flag.String("exclude", "", "comma-separated path patterns to skip")
		maxSize = flag.Int64("max-size", conflictmarkers.DefaultMaxSize, "largest file to read, in bytes")
		root    = flag.String("root", ".", "directory to walk when no files are given")
	)
	flag.Parse()

	opts.Exclude = split(*exclude)
	opts.MaxSize = *maxSize

	findings, err := conflictmarkers.Check(*root, flag.Args(), opts)
	if err != nil {
		fmt.Fprintln(os.Stderr, "check-conflict-markers:", err)
		os.Exit(exitFailure)
	}
	if len(findings) == 0 {
		os.Exit(exitOK)
	}

	for _, f := range findings {
		fmt.Fprintln(os.Stderr, f)
	}
	fmt.Fprintf(os.Stderr, "\n%d conflict marker(s) found. Resolve the conflict and stage the result;\n"+
		"if a marker belongs in the file, pass its path to -exclude.\n", len(findings))
	os.Exit(exitViolation)
}

// split turns a comma-separated flag value into its non-empty parts.
func split(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}
