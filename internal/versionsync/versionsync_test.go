package versionsync

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseVersion(t *testing.T) {
	tests := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{in: "1.22", want: "1.22.0"},
		{in: "1.22.3", want: "1.22.3"},
		{in: "go1.22.3", want: "1.22.3"},
		{in: " 1.26.1 ", want: "1.26.1"},
		{in: "1", wantErr: true},
		{in: "1.2.3.4", wantErr: true},
		{in: "1.x.3", wantErr: true},
		{in: "", wantErr: true},
	}
	for _, tt := range tests {
		got, err := ParseVersion(tt.in)
		if tt.wantErr {
			if err == nil {
				t.Errorf("ParseVersion(%q): want error, got %v", tt.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseVersion(%q): %v", tt.in, err)
			continue
		}
		if got.String() != tt.want {
			t.Errorf("ParseVersion(%q) = %s, want %s", tt.in, got, tt.want)
		}
	}
}

func TestVersionEqualTreatsMissingPatchAsZero(t *testing.T) {
	a, err := ParseVersion("1.22")
	if err != nil {
		t.Fatal(err)
	}
	b, err := ParseVersion("1.22.0")
	if err != nil {
		t.Fatal(err)
	}
	if !a.Equal(b) {
		t.Errorf("1.22 should equal 1.22.0")
	}
}

// write creates a file under dir, making parents as needed.
func write(t *testing.T, dir, rel, content string) string {
	t.Helper()
	path := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestCheckInSync(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "go.mod", "module example.com/x\n\ngo 1.22.3\n")
	write(t, dir, ".tool-versions", "golang 1.22.3\n")

	got, err := Check(dir, ModeExact)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("want no mismatches, got %v", got)
	}
}

func TestCheckDrift(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "go.mod", "module example.com/x\n\ngo 1.26.1\n")
	write(t, dir, ".tool-versions", "# comment\ngolang 1.26.2\ngoreleaser 2.15.3\n")

	got, err := Check(dir, ModeExact)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 mismatch, got %d: %v", len(got), got)
	}
	if got[0].Mod.String() != "1.26.1" || got[0].Pin.String() != "1.26.2" {
		t.Errorf("unexpected mismatch: %+v", got[0])
	}
}

func TestCheckSkipsModuleWithNoGoverningPin(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "go.mod", "module example.com/x\n\ngo 1.22.3\n")

	got, err := Check(dir, ModeExact)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("a module with no .tool-versions should be skipped, got %v", got)
	}
}

func TestCheckNestedModuleUsesNearestAncestorPin(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "go.mod", "module example.com/root\n\ngo 1.22.0\n")
	write(t, dir, ".tool-versions", "golang 1.22.0\n")
	// Nested module with its own pin — the nearer one governs.
	write(t, dir, "apps/svc/go.mod", "module example.com/svc\n\ngo 1.21.0\n")
	write(t, dir, "apps/svc/.tool-versions", "golang 1.23.0\n")

	got, err := Check(dir, ModeExact)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 mismatch, got %d: %v", len(got), got)
	}
	if got[0].ModFile != "apps/svc/go.mod" || got[0].PinFile != "apps/svc/.tool-versions" {
		t.Errorf("wrong governing pin: %+v", got[0])
	}
}

func TestCheckNestedModuleInheritsRootPin(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, ".tool-versions", "golang 1.22.0\n")
	write(t, dir, "apps/svc/go.mod", "module example.com/svc\n\ngo 1.23.0\n")

	got, err := Check(dir, ModeExact)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 mismatch, got %d: %v", len(got), got)
	}
	if got[0].PinFile != ".tool-versions" {
		t.Errorf("want root pin to govern, got %q", got[0].PinFile)
	}
}

func TestCheckSkipsVendorAndDotDirs(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, ".tool-versions", "golang 1.22.0\n")
	write(t, dir, "go.mod", "module example.com/x\n\ngo 1.22.0\n")
	write(t, dir, "vendor/example.com/dep/go.mod", "module dep\n\ngo 1.11\n")
	write(t, dir, "node_modules/pkg/go.mod", "module pkg\n\ngo 1.11\n")
	write(t, dir, ".cache/go.mod", "module cached\n\ngo 1.11\n")

	got, err := Check(dir, ModeExact)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("vendored/dot-dir modules must be skipped, got %v", got)
	}
}

func TestCheckSkipsModuleWithoutGoDirective(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, ".tool-versions", "golang 1.22.0\n")
	write(t, dir, "go.mod", "module example.com/x\n")

	got, err := Check(dir, ModeExact)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("want no mismatches, got %v", got)
	}
}

func TestCheckIgnoresGolangInsideComment(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "go.mod", "module example.com/x\n\ngo 1.22.0\n")
	write(t, dir, ".tool-versions", "# golang 9.9.9 is not the pin\ngolang 1.22.0\n")

	got, err := Check(dir, ModeExact)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("commented golang line must be ignored, got %v", got)
	}
}

func TestFixRewritesOnlyTheGolangLine(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "go.mod", "module example.com/x\n\ngo 1.26.1\n")
	pin := write(t, dir, ".tool-versions",
		"# managed by asdf\n\ngolang 1.26.2\n\n# release tooling\ngoreleaser 2.15.3\n")

	got, err := Check(dir, ModeExact)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 mismatch, got %d", len(got))
	}
	if err := Fix(dir, got[0]); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(pin)
	if err != nil {
		t.Fatal(err)
	}
	want := "# managed by asdf\n\ngolang 1.26.1\n\n# release tooling\ngoreleaser 2.15.3\n"
	if string(data) != want {
		t.Errorf("Fix rewrote more than the golang line:\ngot:  %q\nwant: %q", data, want)
	}

	after, err := Check(dir, ModeExact)
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != 0 {
		t.Errorf("Fix did not converge: %v", after)
	}
}

func TestFixPreservesFileMode(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "go.mod", "module example.com/x\n\ngo 1.26.1\n")
	pin := write(t, dir, ".tool-versions", "golang 1.26.2\n")
	if err := os.Chmod(pin, 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := Check(dir, ModeExact)
	if err != nil {
		t.Fatal(err)
	}
	if err := Fix(dir, got[0]); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(pin)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("mode = %v, want 0600", perm)
	}
}

func TestModeMinAcceptsNewerPin(t *testing.T) {
	dir := t.TempDir()
	// A library: go.mod states a compatibility floor, developed on a newer Go.
	write(t, dir, "go.mod", "module example.com/lib\n\ngo 1.22.0\n")
	write(t, dir, ".tool-versions", "golang 1.26.1\n")

	if got, err := Check(dir, ModeMin); err != nil {
		t.Fatal(err)
	} else if len(got) != 0 {
		t.Errorf("min mode should accept a newer pin, got %v", got)
	}

	// The same tree is a mismatch under exact.
	if got, err := Check(dir, ModeExact); err != nil {
		t.Fatal(err)
	} else if len(got) != 1 {
		t.Errorf("exact mode should flag it, got %d mismatches", len(got))
	}
}

func TestModeMinRejectsOlderPin(t *testing.T) {
	dir := t.TempDir()
	// The pin cannot build the module at all.
	write(t, dir, "go.mod", "module example.com/lib\n\ngo 1.26.0\n")
	write(t, dir, ".tool-versions", "golang 1.22.0\n")

	got, err := Check(dir, ModeMin)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 mismatch, got %d", len(got))
	}
	if !strings.Contains(got[0].String(), "older") {
		t.Errorf("min-mode message should say the pin is older, got %q", got[0])
	}
}

func TestModeMinEqualIsFine(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "go.mod", "module example.com/lib\n\ngo 1.22.0\n")
	write(t, dir, ".tool-versions", "golang 1.22\n")

	if got, err := Check(dir, ModeMin); err != nil {
		t.Fatal(err)
	} else if len(got) != 0 {
		t.Errorf("equal versions satisfy min mode, got %v", got)
	}
}

func TestCompareOrdersByComponent(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"1.22.0", "1.22.0", 0},
		{"1.22", "1.22.0", 0},
		{"1.22.1", "1.22.0", 1},
		{"1.23.0", "1.22.9", 1},
		{"2.0.0", "1.99.99", 1},
		{"1.22.0", "1.22.1", -1},
	}
	for _, tt := range tests {
		a, err := ParseVersion(tt.a)
		if err != nil {
			t.Fatal(err)
		}
		b, err := ParseVersion(tt.b)
		if err != nil {
			t.Fatal(err)
		}
		if got := a.Compare(b); got != tt.want {
			t.Errorf("Compare(%s, %s) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestParseMode(t *testing.T) {
	for _, s := range []string{"exact", "min"} {
		if _, err := ParseMode(s); err != nil {
			t.Errorf("ParseMode(%q): %v", s, err)
		}
	}
	if _, err := ParseMode("loose"); err == nil {
		t.Error("want error for unknown mode")
	}
}

func TestGoDirectiveMalformed(t *testing.T) {
	dir := t.TempDir()
	mod := write(t, dir, "go.mod", "module example.com/x\n\ngo not-a-version\n")

	if _, _, err := GoDirective(mod); err == nil {
		t.Error("want error for malformed go directive")
	}
}
