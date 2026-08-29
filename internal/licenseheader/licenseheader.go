// Package licenseheader checks that source files carry an SPDX header, and
// optionally inserts one.
//
// A repository's root LICENSE governs the repository. It does not travel with
// a file that is copied out of it, which is exactly the case where attribution
// matters most — so the per-file header, not the LICENSE, is what keeps a
// notice attached to the work. This package enforces the short form
// standardized by SPDX and used by the Linux kernel and the REUSE spec, rather
// than a license's full boilerplate:
//
//	// SPDX-FileCopyrightText: 2026 Jane Doe
//	// SPDX-License-Identifier: Apache-2.0
package licenseheader

import (
	"bufio"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Tag names, spelled once.
const (
	TagLicense   = "SPDX-License-Identifier:"
	TagCopyright = "SPDX-FileCopyrightText:"
)

// DefaultHeadLines is how far into a file the header is looked for. Enough to
// clear a build constraint and a blank line without scanning whole files.
const DefaultHeadLines = 5

// Options configures a check. The zero value is not useful; see [Default].
type Options struct {
	// License is the SPDX identifier every file must declare, such as
	// "Apache-2.0". Empty accepts any identifier, which is what a repository
	// with mixed licensing wants.
	License string

	// RequireCopyright demands an SPDX-FileCopyrightText line as well.
	RequireCopyright bool

	// CopyrightText is the text written after the copyright tag by Fix, such
	// as "2026 Jane Doe". Fix reports an error without it.
	CopyrightText string

	// Exts are the file extensions checked when walking a tree, including
	// the dot. Ignored when explicit paths are given.
	Exts []string

	// HeadLines is how many leading lines are searched. Zero means
	// [DefaultHeadLines].
	HeadLines int

	// Exclude holds filepath.Match patterns tested against each path and
	// against its base name. Matching files are skipped.
	Exclude []string
}

// Default returns options that check Go files for any SPDX identifier.
func Default() Options {
	return Options{
		RequireCopyright: true,
		Exts:             []string{".go"},
		HeadLines:        DefaultHeadLines,
	}
}

func (o Options) headLines() int {
	if o.HeadLines > 0 {
		return o.HeadLines
	}
	return DefaultHeadLines
}

// Violation is one file that does not carry the required header.
type Violation struct {
	Path   string
	Reason string
}

func (v Violation) String() string { return v.Path + ": " + v.Reason }

// Check reports every file that lacks the required header.
//
// When paths is non-empty those files are checked directly, which is how
// pre-commit invokes this. Otherwise root is walked for files matching
// Options.Exts.
func Check(root string, paths []string, opts Options) ([]Violation, error) {
	files, err := resolve(root, paths, opts)
	if err != nil {
		return nil, err
	}
	var out []Violation
	for _, f := range files {
		v, err := checkFile(f, opts)
		if err != nil {
			return nil, err
		}
		if v != nil {
			out = append(out, *v)
		}
	}
	return out, nil
}

// Fix inserts the missing header into every file that lacks one, and returns
// the files it rewrote.
func Fix(root string, paths []string, opts Options) ([]string, error) {
	if opts.License == "" {
		return nil, errors.New("cannot insert a header without a license identifier")
	}
	if opts.RequireCopyright && opts.CopyrightText == "" {
		return nil, errors.New("cannot insert a copyright line without copyright text")
	}
	bad, err := Check(root, paths, opts)
	if err != nil {
		return nil, err
	}
	var fixed []string
	for _, v := range bad {
		if err := insert(v.Path, opts); err != nil {
			return fixed, err
		}
		fixed = append(fixed, v.Path)
	}
	return fixed, nil
}

// Header is the block Fix inserts.
func Header(opts Options) string {
	var b strings.Builder
	if opts.RequireCopyright {
		fmt.Fprintf(&b, "// %s %s\n", TagCopyright, opts.CopyrightText)
	}
	fmt.Fprintf(&b, "// %s %s\n", TagLicense, opts.License)
	return b.String()
}

func resolve(root string, paths []string, opts Options) ([]string, error) {
	if len(paths) > 0 {
		var out []string
		for _, p := range paths {
			if !opts.excluded(p) {
				out = append(out, p)
			}
		}
		return out, nil
	}
	var out []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			// Neither ours nor built by us; walking them produces noise and,
			// in the case of testdata, false positives.
			switch d.Name() {
			case ".git", "vendor", "node_modules":
				return fs.SkipDir
			}
			return nil
		}
		if !opts.matchesExt(p) || opts.excluded(p) {
			return nil
		}
		out = append(out, p)
		return nil
	})
	return out, err
}

func (o Options) matchesExt(p string) bool {
	if len(o.Exts) == 0 {
		return true
	}
	ext := filepath.Ext(p)
	for _, want := range o.Exts {
		if strings.EqualFold(ext, want) {
			return true
		}
	}
	return false
}

func (o Options) excluded(p string) bool {
	for _, pat := range o.Exclude {
		if ok, _ := filepath.Match(pat, p); ok {
			return true
		}
		if ok, _ := filepath.Match(pat, filepath.Base(p)); ok {
			return true
		}
	}
	return false
}

// checkFile returns nil when the file is acceptable.
func checkFile(path string, opts Options) (*Violation, error) {
	head, err := readHead(path, opts.headLines())
	if err != nil {
		return nil, err
	}
	// A generated file's first lines belong to whatever produced it, and
	// rewriting them on every regeneration is churn nobody reviews.
	if isGenerated(head) {
		return nil, nil
	}
	lic, ok := find(head, TagLicense)
	switch {
	case !ok:
		return &Violation{path, "missing " + TagLicense}, nil
	case opts.License != "" && lic != opts.License:
		return &Violation{path, fmt.Sprintf("declares %q, want %q", lic, opts.License)}, nil
	}
	if opts.RequireCopyright {
		if _, ok := find(head, TagCopyright); !ok {
			return &Violation{path, "missing " + TagCopyright}, nil
		}
	}
	return nil, nil
}

func readHead(path string, n int) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var out []string
	sc := bufio.NewScanner(f)
	for len(out) < n && sc.Scan() {
		out = append(out, sc.Text())
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// find returns the text after tag, on the first line carrying it.
func find(lines []string, tag string) (string, bool) {
	for _, l := range lines {
		if i := strings.Index(l, tag); i >= 0 {
			return strings.TrimSpace(l[i+len(tag):]), true
		}
	}
	return "", false
}

func isGenerated(lines []string) bool {
	for _, l := range lines {
		t := strings.TrimSpace(l)
		if strings.HasPrefix(t, "// Code generated ") && strings.HasSuffix(t, " DO NOT EDIT.") {
			return true
		}
	}
	return false
}

// insert prepends the header, keeping it above anything already at the top.
//
// A Go build constraint has to precede the package clause and be followed by a
// blank line; putting the header above it with a blank line after satisfies
// both rules, so this is safe for constrained files as well as plain ones.
func insert(path string, opts Options) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	out := append([]byte(Header(opts)+"\n"), body...)
	return os.WriteFile(path, out, info.Mode().Perm())
}
