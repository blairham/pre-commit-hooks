// Package conflictmarkers finds version-control conflict markers that have
// been committed into a file.
//
// The markers are the four git writes into a conflicted file: the ours line,
// the base line that only the diff3 and zdiff3 conflict styles produce, the
// separator, and the theirs line.
//
// Two decisions shape the whole package.
//
// The base marker is checked. Tooling that looks only for the three markers of
// the default conflict style will pass a file that a diff3 merge left behind,
// because the base line is the one marker whose style is not universal.
//
// The separator is only a conflict when the file carries one of the other
// three. On its own a row of equals signs is ordinary prose: it underlines a
// setext heading in Markdown and a section title in reStructuredText, and it
// rules off a block of plain text. That ambiguity is why conflict checks tend
// to run only while a merge is in progress — which is precisely when a marker
// that was already committed is no longer reachable. Reading the separator in
// the context of its file keeps the check running at every commit without
// failing on a heading.
package conflictmarkers

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// The marker lines git writes into a conflicted file. Each is exactly seven
// characters; anything longer is not a marker, so the run length is matched
// exactly rather than as a minimum.
const (
	MarkerOurs      = "<<<<<<<"
	MarkerBase      = "|||||||"
	MarkerSeparator = "======="
	MarkerTheirs    = ">>>>>>>"
)

// markerRunLength is the length git gives every conflict marker.
const markerRunLength = 7

// DefaultMaxSize is the largest file the check will read. A file past it is
// skipped rather than reported: conflict markers live in text a human merged,
// and reading an arbitrarily large blob to look for one costs more than it
// finds.
const DefaultMaxSize = 10 << 20 // 10 MiB

// Options configures a check.
type Options struct {
	// Exclude holds path patterns; a file whose slash-separated path contains
	// one of them as a substring is skipped. A file that documents conflict
	// markers, or a fixture that deliberately carries one, belongs here.
	Exclude []string

	// MaxSize is the largest file to read, in bytes. Zero means DefaultMaxSize.
	MaxSize int64
}

// Default returns the options the command line starts from.
func Default() Options {
	return Options{MaxSize: DefaultMaxSize}
}

// Finding is one marker line in one file.
type Finding struct {
	Path   string
	Line   int    // 1-based
	Marker string // one of the four Marker constants
}

// String renders a finding as one diagnostic line.
func (f Finding) String() string {
	return fmt.Sprintf("%s:%d: conflict marker %s", f.Path, f.Line, f.Marker)
}

// Check reports every conflict marker in the given files. When paths is empty
// it walks root instead, skipping any .git directory.
//
// A file that cannot be read is an error; a file that is binary, oversized, or
// excluded is skipped without one. Findings come back in the order the files
// were given, and by line within a file.
func Check(root string, paths []string, opts Options) ([]Finding, error) {
	if opts.MaxSize <= 0 {
		opts.MaxSize = DefaultMaxSize
	}

	if len(paths) == 0 {
		walked, err := walk(root)
		if err != nil {
			return nil, err
		}
		paths = walked
	}

	var findings []Finding
	for _, path := range paths {
		if excluded(path, opts.Exclude) {
			continue
		}
		found, err := checkFile(path, opts)
		if err != nil {
			return nil, err
		}
		findings = append(findings, found...)
	}
	return findings, nil
}

// checkFile reports the markers in one file.
func checkFile(path string, opts Options) ([]Finding, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if info.IsDir() || !info.Mode().IsRegular() || info.Size() > opts.MaxSize {
		return nil, nil
	}

	data, err := os.ReadFile(path) //nolint:gosec // the path is one the caller asked to check
	if err != nil {
		return nil, err
	}
	if bytes.IndexByte(data, 0) >= 0 {
		return nil, nil // binary
	}

	var (
		candidates []Finding
		vouched    bool
	)
	for i, line := range strings.Split(string(data), "\n") {
		marker, ok := markerOn(line)
		if !ok {
			continue
		}
		if marker != MarkerSeparator {
			vouched = true
		}
		candidates = append(candidates, Finding{Path: path, Line: i + 1, Marker: marker})
	}

	// A separator alone is prose — a heading underline or a rule. It only
	// becomes evidence of a conflict once another marker vouches for the file.
	if vouched {
		return candidates, nil
	}
	var kept []Finding
	for _, c := range candidates {
		if c.Marker != MarkerSeparator {
			kept = append(kept, c)
		}
	}
	return kept, nil
}

// markerOn reports the conflict marker a line opens with, if any.
//
// A marker is exactly seven of its character at the start of the line,
// followed by the end of the line or by a space — the shape git writes, and
// the shape that keeps a longer run of the same character from matching.
func markerOn(line string) (string, bool) {
	line = strings.TrimSuffix(line, "\r")
	if len(line) < markerRunLength {
		return "", false
	}
	marker := line[:markerRunLength]
	switch marker {
	case MarkerOurs, MarkerBase, MarkerSeparator, MarkerTheirs:
	default:
		return "", false
	}
	if len(line) > markerRunLength && line[markerRunLength] != ' ' {
		return "", false
	}
	return marker, true
}

// excluded reports whether a path matches one of the exclusion patterns.
func excluded(path string, patterns []string) bool {
	slashed := filepath.ToSlash(path)
	for _, p := range patterns {
		if p != "" && strings.Contains(slashed, p) {
			return true
		}
	}
	return false
}

// walk collects the regular files under root, skipping any .git directory.
func walk(root string) ([]string, error) {
	var paths []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return fs.SkipDir
			}
			return nil
		}
		if d.Type().IsRegular() {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return paths, nil
}
