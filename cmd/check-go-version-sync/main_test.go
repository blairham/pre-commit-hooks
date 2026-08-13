package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/pre-commit-hooks/internal/versionsync"
)

func write(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestRunInSync(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "go.mod", "module example.com/x\n\ngo 1.22.0\n")
	write(t, dir, ".tool-versions", "golang 1.22.0\n")

	var out bytes.Buffer
	if code := run(&out, dir, versionsync.ModeExact, false); code != exitOK {
		t.Errorf("exit = %d, want %d (output: %s)", code, exitOK, out.String())
	}
	if out.Len() != 0 {
		t.Errorf("want silence on success, got %q", out.String())
	}
}

func TestRunReportsDrift(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "go.mod", "module example.com/x\n\ngo 1.26.1\n")
	write(t, dir, ".tool-versions", "golang 1.26.2\n")

	var out bytes.Buffer
	if code := run(&out, dir, versionsync.ModeExact, false); code != exitDrift {
		t.Errorf("exit = %d, want %d", code, exitDrift)
	}
	if !strings.Contains(out.String(), "1.26.1") || !strings.Contains(out.String(), "1.26.2") {
		t.Errorf("output should name both versions, got %q", out.String())
	}
}

func TestRunFixConverges(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "go.mod", "module example.com/x\n\ngo 1.26.1\n")
	write(t, dir, ".tool-versions", "golang 1.26.2\n")

	var out bytes.Buffer
	// -fix still exits non-zero so the commit stops for staging.
	if code := run(&out, dir, versionsync.ModeExact, true); code != exitDrift {
		t.Errorf("exit = %d, want %d", code, exitDrift)
	}
	if !strings.Contains(out.String(), "fixed:") {
		t.Errorf("want a fixed: line, got %q", out.String())
	}

	// A second pass is clean.
	out.Reset()
	if code := run(&out, dir, versionsync.ModeExact, false); code != exitOK {
		t.Errorf("second pass exit = %d, want %d (output: %s)", code, exitOK, out.String())
	}
}

func TestRunUnreadableRootFails(t *testing.T) {
	var out bytes.Buffer
	if code := run(&out, filepath.Join(t.TempDir(), "does-not-exist"), versionsync.ModeExact, false); code != exitFailure {
		t.Errorf("exit = %d, want %d", code, exitFailure)
	}
}
