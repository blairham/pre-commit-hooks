// Command check-license-headers fails when a source file does not carry an
// SPDX license header, so that attribution stays attached to a file that is
// copied out of the repository rather than only to the repository.
//
// It is designed to run as a pre-commit hook but works standalone:
//
//	check-license-headers -license Apache-2.0
//	check-license-headers -license MIT -copyright-text '2026 Jane Doe' -fix
//	check-license-headers -ext .go,.rs -root ./src
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/blairham/pre-commit-hooks/internal/licenseheader"
)

const (
	exitOK        = 0
	exitViolation = 1
	exitFailure   = 2
)

func main() {
	opts := licenseheader.Default()
	var (
		license   = flag.String("license", "", "required SPDX-License-Identifier, e.g. Apache-2.0 (empty: accept any)")
		copyText  = flag.String("copyright-text", "", "text written after "+licenseheader.TagCopyright+" by -fix, e.g. '2026 Jane Doe'")
		needCopy  = flag.Bool("copyright", true, "also require an "+licenseheader.TagCopyright+" line")
		exts      = flag.String("ext", ".go", "comma-separated file extensions to check when walking")
		exclude   = flag.String("exclude", "", "comma-separated path patterns to skip")
		headLines = flag.Int("head", licenseheader.DefaultHeadLines, "how many leading lines to search")
		root      = flag.String("root", ".", "directory to walk when no files are given")
		fix       = flag.Bool("fix", false, "insert the missing header instead of failing")
	)
	flag.Parse()

	opts.License = *license
	opts.CopyrightText = *copyText
	opts.RequireCopyright = *needCopy
	opts.Exts = split(*exts)
	opts.Exclude = split(*exclude)
	opts.HeadLines = *headLines

	if err := run(opts, *root, flag.Args(), *fix); err != nil {
		fmt.Fprintln(os.Stderr, "check-license-headers:", err)
		os.Exit(exitFailure)
	}
}

func run(opts licenseheader.Options, root string, paths []string, fix bool) error {
	if fix {
		fixed, err := licenseheader.Fix(root, paths, opts)
		if err != nil {
			return err
		}
		for _, f := range fixed {
			fmt.Println("added header:", f)
		}
		if len(fixed) > 0 {
			// Same contract as the other hooks here: a fix is a change to
			// the working tree, so the run still fails and the commit is
			// retried against the rewritten files.
			os.Exit(exitViolation)
		}
		return nil
	}

	bad, err := licenseheader.Check(root, paths, opts)
	if err != nil {
		return err
	}
	if len(bad) == 0 {
		return nil
	}
	for _, v := range bad {
		fmt.Fprintln(os.Stderr, v)
	}
	fmt.Fprint(os.Stderr, advice(opts))
	os.Exit(exitViolation)
	return nil
}

// advice prints the header the run wanted, because the useful thing to show
// someone who is missing a header is the header.
func advice(opts licenseheader.Options) string {
	var b strings.Builder
	b.WriteString("\nA repository's LICENSE does not travel with a file copied out of it.\n")
	b.WriteString("The header does. Add to the top of each file listed above:\n\n")
	shown := opts
	if shown.License == "" {
		shown.License = "<SPDX-ID>"
	}
	if shown.CopyrightText == "" {
		shown.CopyrightText = "<year> <owner>"
	}
	for _, l := range strings.Split(strings.TrimRight(licenseheader.Header(shown), "\n"), "\n") {
		b.WriteString("  " + l + "\n")
	}
	b.WriteString("\nOr run with -fix to insert it.\n")
	return b.String()
}

func split(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
