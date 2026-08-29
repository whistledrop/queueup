package game

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeFile(t *testing.T, path string, mod time.Time) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("log"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, mod, mod); err != nil {
		t.Fatal(err)
	}
}

// The case that burned a real afternoon: the AppData folder exists (the game
// keeps its menu settings there) but the log itself lives in the install
// folder, because Rust launches with -logfile output_log.txt.
func TestInstallFolderLogWinsOverAnEmptyAppDataFolder(t *testing.T) {
	home := t.TempDir()
	install := t.TempDir()

	// AppData has the folder structure but NO log, exactly like the real PC.
	if err := os.MkdirAll(filepath.Join(home, "AppData", "LocalLow", "Facepunch Studios LTD", "Rust"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(install, "output_log.txt"), time.Now())

	got := FindRustLog(home, []string{install})
	if got != filepath.Join(install, "output_log.txt") {
		t.Fatalf("FindRustLog = %q, want the install folder's output_log.txt", got)
	}
}

func TestLegacyPlayerLogStillFoundWhenItIsTheOnlyLog(t *testing.T) {
	home := t.TempDir()
	legacy := filepath.Join(home, "AppData", "LocalLow", "Facepunch Studios LTD", "Rust", "Player.log")
	writeFile(t, legacy, time.Now())

	if got := FindRustLog(home, nil); got != legacy {
		t.Fatalf("FindRustLog = %q, want the legacy Player.log", got)
	}
}

// Both exist (an old install upgraded in place): the one the game is actually
// writing, the fresher one, wins.
func TestTheFresherLogWinsWhenBothExist(t *testing.T) {
	home := t.TempDir()
	install := t.TempDir()
	stale := filepath.Join(home, "AppData", "LocalLow", "Facepunch Studios LTD", "Rust", "Player.log")
	fresh := filepath.Join(install, "output_log.txt")
	writeFile(t, stale, time.Now().Add(-30*24*time.Hour))
	writeFile(t, fresh, time.Now())

	if got := FindRustLog(home, []string{install}); got != fresh {
		t.Fatalf("FindRustLog = %q, want the fresh install log", got)
	}

	// And the other way round, in case someone's install is genuinely legacy.
	writeFile(t, stale, time.Now().Add(time.Hour))
	if got := FindRustLog(home, []string{install}); got != stale {
		t.Fatalf("FindRustLog = %q, want the fresher legacy log", got)
	}
}

// A machine where Rust has never run: watch where the game WILL write, which is
// the install folder when Steam told us where that is.
func TestAFreshInstallWatchesTheInstallFolder(t *testing.T) {
	home := t.TempDir()
	install := t.TempDir()
	if got := FindRustLog(home, []string{install}); got != filepath.Join(install, "output_log.txt") {
		t.Fatalf("FindRustLog = %q, want the future output_log.txt", got)
	}
}
