package game

import (
	"os"
	"path/filepath"
	"testing"
)

// The company folder is "Facepunch Studios LTD", the studio's legal name, not
// "Facepunch". That was assumed wrong for the whole build and only a real PC
// caught it, which is why this is now discovered rather than hard coded.
func TestFindRustLogUsesTheFolderThatActuallyExists(t *testing.T) {
	home := t.TempDir()
	real := filepath.Join(home, "AppData", "LocalLow", "Facepunch Studios LTD", "Rust")
	if err := os.MkdirAll(real, 0o755); err != nil {
		t.Fatal(err)
	}

	got := FindRustLog(home)
	want := filepath.Join(real, "Player.log")
	if got != want {
		t.Fatalf("FindRustLog = %q, want %q", got, want)
	}
}

// If a future Rust ships under a different Facepunch folder, that must be found
// too, without another emergency fix on a wipe day.
func TestFindRustLogAcceptsAnyFacepunchFolder(t *testing.T) {
	for _, company := range []string{"Facepunch", "Facepunch Studios", "facepunch studios ltd"} {
		home := t.TempDir()
		dir := filepath.Join(home, "AppData", "LocalLow", company, "Rust")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if got, want := FindRustLog(home), filepath.Join(dir, "Player.log"); got != want {
			t.Errorf("with company %q: FindRustLog = %q, want %q", company, got, want)
		}
	}
}

// A folder called Facepunch-something with no Rust inside must not win over
// the one that has it.
func TestFindRustLogIgnoresAFacepunchFolderWithoutRust(t *testing.T) {
	home := t.TempDir()
	low := filepath.Join(home, "AppData", "LocalLow")
	if err := os.MkdirAll(filepath.Join(low, "Facepunch Other Game", "Sandbox"), 0o755); err != nil {
		t.Fatal(err)
	}
	real := filepath.Join(low, "Facepunch Studios LTD", "Rust")
	if err := os.MkdirAll(real, 0o755); err != nil {
		t.Fatal(err)
	}
	if got, want := FindRustLog(home), filepath.Join(real, "Player.log"); got != want {
		t.Fatalf("FindRustLog = %q, want %q", got, want)
	}
}

// Before Rust has ever run there is nothing to find. That is normal, and the
// answer must still be a sensible path for the tailer to watch, never empty.
func TestFindRustLogBeforeRustHasEverRun(t *testing.T) {
	home := t.TempDir()
	got := FindRustLog(home)
	want := filepath.Join(home, "AppData", "LocalLow", "Facepunch Studios LTD", "Rust", "Player.log")
	if got != want {
		t.Fatalf("FindRustLog on a fresh PC = %q, want %q", got, want)
	}
}

func TestFindRustLogWithNoHome(t *testing.T) {
	if got := FindRustLog(""); got != "" {
		t.Fatalf("FindRustLog(\"\") = %q, want empty", got)
	}
}
