// Package versionsync compares the Go version a module declares against the
// version its governing .tool-versions file pins, so a repo's asdf toolchain
// and its go.mod can't drift apart.
//
// The rule: for each go.mod in the tree, the governing pin is the nearest
// ancestor .tool-versions carrying a `golang` line. A module with no governing
// pin is skipped rather than failed — not every repo uses asdf.
package versionsync

import (
	"bufio"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Version is a parsed Go version. A missing patch component is zero, so
// `go 1.22` and `golang 1.22.0` compare equal.
//
// HasPatch records whether the patch component was written explicitly. It
// matters because a go directive of `go 1.26` expresses no patch requirement
// at all — it is a language version, not a toolchain pin — so it must not be
// read as demanding exactly 1.26.0.
type Version struct {
	Major, Minor, Patch int
	HasPatch            bool
	Raw                 string
}

// ParseVersion parses "1.22" or "1.22.3" (a leading "go" is tolerated, as in
// go.mod's `toolchain go1.22.3`).
func ParseVersion(s string) (Version, error) {
	raw := s
	s = strings.TrimPrefix(strings.TrimSpace(s), "go")

	parts := strings.Split(s, ".")
	if len(parts) < 2 || len(parts) > 3 {
		return Version{}, fmt.Errorf("malformed Go version %q", raw)
	}

	v := Version{Raw: raw, HasPatch: len(parts) == 3}
	dst := []*int{&v.Major, &v.Minor, &v.Patch}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return Version{}, fmt.Errorf("malformed Go version %q", raw)
		}
		*dst[i] = n
	}
	return v, nil
}

// Equal reports whether two versions refer to the same release.
func (v Version) Equal(o Version) bool {
	return v.Compare(o) == 0
}

// Compare returns -1 if v sorts before o, 0 if equal, +1 if after.
func (v Version) Compare(o Version) int {
	for _, pair := range [][2]int{
		{v.Major, o.Major},
		{v.Minor, o.Minor},
		{v.Patch, o.Patch},
	} {
		switch {
		case pair[0] < pair[1]:
			return -1
		case pair[0] > pair[1]:
			return 1
		}
	}
	return 0
}

// String renders the canonical major.minor.patch form.
func (v Version) String() string {
	return fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
}

// Mode selects how a module's go directive is compared against its pin.
type Mode string

const (
	// ModeExact requires the pin to equal the go directive. Right for
	// applications and CLIs, where the pinned toolchain is the one that builds
	// the artifact you ship, and drift means local and CI differ.
	ModeExact Mode = "exact"

	// ModeMin requires only that the pin satisfies the go directive
	// (pin >= directive). Right for libraries and reusable modules, where the
	// go directive is a compatibility floor for consumers and the toolchain you
	// develop with is legitimately newer.
	ModeMin Mode = "min"
)

// ParseMode validates a mode name.
func ParseMode(s string) (Mode, error) {
	switch Mode(s) {
	case ModeExact:
		return ModeExact, nil
	case ModeMin:
		return ModeMin, nil
	default:
		return "", fmt.Errorf("unknown mode %q (want %q or %q)", s, ModeExact, ModeMin)
	}
}

// Mismatch is one module whose declared Go version disagrees with its pin.
type Mismatch struct {
	ModFile string // path to the go.mod
	PinFile string // path to the governing .tool-versions
	Mod     Version
	Pin     Version
	Mode    Mode
}

func (m Mismatch) String() string {
	if m.Mode == ModeMin {
		return fmt.Sprintf("%s declares 'go %s' but %s pins an older 'golang %s'",
			m.ModFile, m.Mod.Raw, m.PinFile, m.Pin.Raw)
	}
	return fmt.Sprintf("%s declares 'go %s' but %s pins 'golang %s'",
		m.ModFile, m.Mod.Raw, m.PinFile, m.Pin.Raw)
}

// satisfied reports whether a pin is acceptable for a go directive under mode.
//
// A go directive written without a patch (`go 1.26`) is a language version, not
// a toolchain pin: it says nothing about which patch to build with. Exact mode
// therefore compares only major.minor in that case, so `go 1.26` accepts a
// `golang 1.26.1` pin. Min mode already treats the missing patch as a .0 floor.
func satisfied(mode Mode, mod, pin Version) bool {
	if mode == ModeMin {
		return pin.Compare(mod) >= 0
	}
	if !mod.HasPatch {
		return pin.Major == mod.Major && pin.Minor == mod.Minor
	}
	return pin.Equal(mod)
}

// GoDirective reads the `go` directive from a go.mod. It returns ok=false when
// the file has no such directive.
func GoDirective(modFile string) (Version, bool, error) {
	f, err := os.Open(modFile)
	if err != nil {
		return Version{}, false, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) >= 2 && fields[0] == "go" {
			v, err := ParseVersion(fields[1])
			if err != nil {
				return Version{}, false, fmt.Errorf("%s: %w", modFile, err)
			}
			return v, true, nil
		}
	}
	if err := scanner.Err(); err != nil {
		return Version{}, false, fmt.Errorf("%s: %w", modFile, err)
	}
	return Version{}, false, nil
}

// GolangPin reads the `golang` line from a .tool-versions file. asdf permits
// several versions on one line; the first is the active one.
func GolangPin(pinFile string) (Version, bool, error) {
	f, err := os.Open(pinFile)
	if err != nil {
		return Version{}, false, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if i := strings.IndexByte(line, '#'); i >= 0 {
			line = line[:i]
		}
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] == "golang" {
			v, err := ParseVersion(fields[1])
			if err != nil {
				return Version{}, false, fmt.Errorf("%s: %w", pinFile, err)
			}
			return v, true, nil
		}
	}
	if err := scanner.Err(); err != nil {
		return Version{}, false, fmt.Errorf("%s: %w", pinFile, err)
	}
	return Version{}, false, nil
}

// FindModules walks root for go.mod files, skipping vendor, node_modules, and
// dot-directories. Paths are returned relative to root, slash-separated.
func FindModules(root string) ([]string, error) {
	var mods []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			name := d.Name()
			if path != root && (name == "vendor" || name == "node_modules" || strings.HasPrefix(name, ".")) {
				return fs.SkipDir
			}
			return nil
		}
		if d.Name() == "go.mod" {
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			mods = append(mods, filepath.ToSlash(rel))
		}
		return nil
	})
	return mods, err
}

// governingPin walks up from the module directory to root looking for the
// nearest .tool-versions with a golang line.
func governingPin(root, modDir string) (Version, string, bool, error) {
	dir := modDir
	for {
		pinFile := filepath.Join(dir, ".tool-versions")
		if _, err := os.Stat(pinFile); err == nil {
			v, ok, err := GolangPin(pinFile)
			if err != nil {
				return Version{}, "", false, err
			}
			if ok {
				rel, err := filepath.Rel(root, pinFile)
				if err != nil {
					return Version{}, "", false, err
				}
				return v, filepath.ToSlash(rel), true, nil
			}
		}
		if dir == root {
			return Version{}, "", false, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return Version{}, "", false, nil
		}
		dir = parent
	}
}

// Check returns every module whose go directive disagrees with its governing
// pin under mode. Modules with no go directive or no governing pin are skipped.
func Check(root string, mode Mode) ([]Mismatch, error) {
	root = filepath.Clean(root)

	mods, err := FindModules(root)
	if err != nil {
		return nil, err
	}

	var mismatches []Mismatch
	for _, rel := range mods {
		modFile := filepath.Join(root, filepath.FromSlash(rel))

		modVer, ok, err := GoDirective(modFile)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}

		pinVer, pinFile, ok, err := governingPin(root, filepath.Dir(modFile))
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}

		if !satisfied(mode, modVer, pinVer) {
			mismatches = append(mismatches, Mismatch{
				ModFile: rel,
				PinFile: pinFile,
				Mod:     modVer,
				Pin:     pinVer,
				Mode:    mode,
			})
		}
	}
	return mismatches, nil
}

// Fix rewrites the golang line of the mismatch's .tool-versions to the version
// the go.mod declares, preserving every other line, comments, and the file mode.
//
// It refuses when the go directive has no patch component: asdf needs a full
// version, so writing `golang 1.26` would leave an unresolvable pin.
func Fix(root string, m Mismatch) error {
	if !m.Mod.HasPatch {
		return fmt.Errorf(
			"%s declares 'go %s' with no patch component; asdf needs a full version, "+
				"so %s cannot be rewritten automatically — set the pin by hand",
			m.ModFile, m.Mod.Raw, m.PinFile)
	}

	pinFile := filepath.Join(root, filepath.FromSlash(m.PinFile))

	info, err := os.Stat(pinFile)
	if err != nil {
		return err
	}

	data, err := os.ReadFile(pinFile)
	if err != nil {
		return err
	}

	lines := strings.Split(string(data), "\n")
	replaced := false
	for i, line := range lines {
		code := line
		if j := strings.IndexByte(code, '#'); j >= 0 {
			code = code[:j]
		}
		fields := strings.Fields(code)
		if len(fields) >= 2 && fields[0] == "golang" {
			lines[i] = "golang " + m.Mod.Raw
			replaced = true
			break
		}
	}
	if !replaced {
		return fmt.Errorf("%s: no golang line to rewrite", m.PinFile)
	}

	return os.WriteFile(pinFile, []byte(strings.Join(lines, "\n")), info.Mode().Perm())
}
