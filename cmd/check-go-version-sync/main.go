// Command check-go-version-sync fails when a go.mod's `go` directive and the
// governing .tool-versions' `golang` pin drift apart, so that asdf and CI build
// the same toolchain.
//
// It is designed to run as a pre-commit hook but works standalone:
//
//	check-go-version-sync            # report drift, exit 1 if any
//	check-go-version-sync -fix       # rewrite .tool-versions from go.mod
//	check-go-version-sync -root path # check a tree other than the cwd
package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/blairham/pre-commit-hooks/internal/versionsync"
)

const (
	exitOK      = 0
	exitDrift   = 1
	exitFailure = 2
)

func main() {
	var (
		fix  = flag.Bool("fix", false, "rewrite .tool-versions to match go.mod instead of failing")
		root = flag.String("root", ".", "root directory to scan for go.mod files")
		mode = flag.String("mode", string(versionsync.ModeExact),
			"exact: pin must equal the go directive (apps); min: pin must be >= it (libraries)")
	)
	flag.Parse()

	m, err := versionsync.ParseMode(*mode)
	if err != nil {
		fmt.Fprintln(os.Stderr, "check-go-version-sync:", err)
		os.Exit(exitFailure)
	}

	os.Exit(run(os.Stderr, *root, m, *fix))
}

func run(out io.Writer, root string, mode versionsync.Mode, fix bool) int {
	mismatches, err := versionsync.Check(root, mode)
	if err != nil {
		fmt.Fprintln(out, "check-go-version-sync:", err)
		return exitFailure
	}
	if len(mismatches) == 0 {
		return exitOK
	}

	for _, m := range mismatches {
		if !fix {
			fmt.Fprintf(out, "ERROR: %s\n", m)
			continue
		}
		if err := versionsync.Fix(root, m); err != nil {
			fmt.Fprintln(out, "check-go-version-sync:", err)
			return exitFailure
		}
		fmt.Fprintf(out, "fixed: %s now pins 'golang %s' (was %s)\n",
			m.PinFile, m.Mod.Raw, m.Pin.Raw)
	}

	if !fix {
		switch mode {
		case versionsync.ModeMin:
			fmt.Fprint(out, "\nThe pinned toolchain is older than the language version go.mod requires,\n"+
				"so this module cannot build with it.\n")
		default:
			fmt.Fprint(out, "\ngo.mod is authoritative — its `go` directive gets pulled up by the\n"+
				"`tool` block during `go mod tidy`, and .tool-versions follows.\n"+
				"(If this is a library whose go directive is a compatibility floor,\n"+
				"use -mode=min instead.)\n")
		}
		fmt.Fprint(out, "Fix with: check-go-version-sync -fix   (or edit both in one commit)\n")
	}

	// Even when -fix succeeds the run still fails, matching how pre-commit
	// treats auto-fixing hooks: the commit stops so the change can be staged.
	return exitDrift
}
