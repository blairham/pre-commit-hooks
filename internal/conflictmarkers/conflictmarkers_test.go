package conflictmarkers

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The markers are built from the constants rather than written out, so this
// file does not itself carry a conflict marker for the hook to find.
var (
	ours  = MarkerOurs
	base  = MarkerBase
	sep   = MarkerSeparator
	their = MarkerTheirs
)

// write puts content in a temporary file and returns its path.
func write(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	return path
}

// markersOf renders findings as "line:marker" for compact comparison.
func markersOf(findings []Finding) []string {
	out := make([]string, 0, len(findings))
	for _, f := range findings {
		out = append(out, f.Marker)
	}
	return out
}

func TestAFullConflictIsReportedIncludingTheBaseMarker(t *testing.T) {
	content := strings.Join([]string{
		"before",
		ours + " ours",
		"mine",
		base + " merged common ancestors",
		"original",
		sep,
		"theirs",
		their + " topic",
		"after",
	}, "\n")

	findings, err := Check("", []string{write(t, "f.txt", content)}, Default())
	if err != nil {
		t.Fatalf("Check: %v", err)
	}

	want := []string{ours, base, sep, their}
	if got := markersOf(findings); !equal(got, want) {
		t.Errorf("markers = %v, want %v", got, want)
	}
	wantLines := []int{2, 4, 6, 8}
	for i, f := range findings {
		if f.Line != wantLines[i] {
			t.Errorf("finding %d line = %d, want %d", i, f.Line, wantLines[i])
		}
	}
}

// The incident this hook exists for: a diff3 merge left its base marker
// behind, and a three-marker check passed the file.
func TestABaseMarkerAloneIsReported(t *testing.T) {
	content := "a\n" + base + " merged common ancestors\nb\n"

	findings, err := Check("", []string{write(t, "spec.md", content)}, Default())
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("findings = %d, want 1: %v", len(findings), findings)
	}
	if findings[0].Marker != base {
		t.Errorf("marker = %q, want %q", findings[0].Marker, base)
	}
	if findings[0].Line != 2 {
		t.Errorf("line = %d, want 2", findings[0].Line)
	}
}

// A row of equals signs underlines a setext heading. Reporting it would make
// the hook unusable on prose, which is why the separator needs a companion.
func TestASeparatorAloneIsProseNotAConflict(t *testing.T) {
	content := "Heading\n" + sep + "\n\nbody text\n"

	findings, err := Check("", []string{write(t, "README.md", content)}, Default())
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if len(findings) != 0 {
		t.Errorf("findings = %v, want none", findings)
	}
}

func TestASeparatorIsReportedOnceAnotherMarkerVouchesForTheFile(t *testing.T) {
	content := "Heading\n" + sep + "\n" + their + " topic\n"

	findings, err := Check("", []string{write(t, "README.md", content)}, Default())
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if got, want := markersOf(findings), []string{sep, their}; !equal(got, want) {
		t.Errorf("markers = %v, want %v", got, want)
	}
}

func TestALongerRunIsNotAMarker(t *testing.T) {
	// Eight characters, not seven: an ASCII rule, not something git wrote.
	content := "a\n" + ours + "<\nb\n" + sep + "=\n"

	findings, err := Check("", []string{write(t, "f.txt", content)}, Default())
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if len(findings) != 0 {
		t.Errorf("findings = %v, want none", findings)
	}
}

func TestAMarkerMustStartTheLine(t *testing.T) {
	content := "quoted: " + ours + " ours\n"

	findings, err := Check("", []string{write(t, "f.txt", content)}, Default())
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if len(findings) != 0 {
		t.Errorf("findings = %v, want none", findings)
	}
}

func TestCarriageReturnsDoNotHideAMarker(t *testing.T) {
	content := "a\r\n" + ours + "\r\n" + their + " topic\r\n"

	findings, err := Check("", []string{write(t, "f.txt", content)}, Default())
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if got, want := markersOf(findings), []string{ours, their}; !equal(got, want) {
		t.Errorf("markers = %v, want %v", got, want)
	}
}

func TestABinaryFileIsSkipped(t *testing.T) {
	content := "a\x00b\n" + ours + " ours\n"

	findings, err := Check("", []string{write(t, "f.bin", content)}, Default())
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if len(findings) != 0 {
		t.Errorf("findings = %v, want none", findings)
	}
}

func TestAnExcludedPathIsSkipped(t *testing.T) {
	path := write(t, "merging.md", ours+" ours\n")

	opts := Default()
	opts.Exclude = []string{"merging.md"}
	findings, err := Check("", []string{path}, opts)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if len(findings) != 0 {
		t.Errorf("findings = %v, want none", findings)
	}
}

func TestAFileOverTheSizeLimitIsSkipped(t *testing.T) {
	path := write(t, "big.txt", ours+" ours\n")

	opts := Default()
	opts.MaxSize = 4
	findings, err := Check("", []string{path}, opts)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if len(findings) != 0 {
		t.Errorf("findings = %v, want none", findings)
	}
}

func TestWalkingSkipsTheGitDirectory(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// git's own files are full of markers during a merge; they are not ours
	// to report.
	if err := os.WriteFile(filepath.Join(root, ".git", "MERGE_MSG"), []byte(ours+" ours\n"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "tracked.txt"), []byte(base+" ancestors\n"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	findings, err := Check(root, nil, Default())
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("findings = %d, want 1: %v", len(findings), findings)
	}
	if filepath.Base(findings[0].Path) != "tracked.txt" {
		t.Errorf("path = %q, want tracked.txt", findings[0].Path)
	}
}

func TestAMissingFileIsAnError(t *testing.T) {
	_, err := Check("", []string{filepath.Join(t.TempDir(), "absent.txt")}, Default())
	if err == nil {
		t.Fatal("Check succeeded on a missing file, want an error")
	}
}

func TestAFindingRendersAsOneDiagnosticLine(t *testing.T) {
	f := Finding{Path: "docs/spec.md", Line: 1990, Marker: base}
	if got, want := f.String(), "docs/spec.md:1990: conflict marker "+base; got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
