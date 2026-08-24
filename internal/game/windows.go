//go:build windows

package game

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// rustProcess is the Rust client's executable name, used only for "is it
// running" and "please close". We never open, read or write the process itself.
const rustProcess = "RustClient.exe"

const steamProcess = "steam.exe"

// WindowsLauncher is the real thing. It is the only file in the project that
// runs on the player's PC and talks to Windows.
//
// NOTE: this is written but not yet proven on a real machine. It gets its first
// real test in phase 5 (see docs/steam-uri-test.md and TESTING.md).
type WindowsLauncher struct {
	// Log defaults to the standard Unity location if empty.
	Log string

	mu      sync.Mutex
	exited  chan Exit
	watch   chan struct{}
	closing bool
}

// DefaultLogPath is where the Rust client writes its log on Windows.
func DefaultLogPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, "AppData", "LocalLow", "Facepunch", "Rust", "Player.log")
}

func (w *WindowsLauncher) LogPath() string {
	if w.Log != "" {
		return w.Log
	}
	return DefaultLogPath()
}

// Preflight catches the failures we can explain properly, before we waste a
// launch attempt on them.
//
// Only genuine blockers belong here. A missing Rust log folder is NOT one: the
// folder is created the first time the game runs, so refusing to launch
// because it is absent is refusing to do the very thing that would fix it.
// That check used to live here and it deadlocked a freshly installed PC.
func (w *WindowsLauncher) Preflight() error {
	if !processRunning(steamProcess) {
		return errors.New("Steam isn't running on your PC. Start Steam and sign in, then try again.")
	}
	return nil
}

// LogFolderExists reports whether Rust has ever run on this machine. Used only
// to explain a quiet log, never to stop a launch.
func (w *WindowsLauncher) LogFolderExists() bool {
	p := w.LogPath()
	if p == "" {
		return false
	}
	_, err := os.Stat(filepath.Dir(p))
	return err == nil
}

// Launch hands the Steam URI to Windows, exactly as if the user had clicked a
// steam:// link in a browser.
//
// If Rust is already running we close it first. The Steam URI does not reliably
// redirect an already-running client to a different server, so the only
// dependable behaviour is: close, then launch fresh. This costs a few seconds
// and is the safe choice on wipe day.
func (w *WindowsLauncher) Launch(a Addr) error {
	if processRunning(rustProcess) {
		if err := w.Close(); err != nil {
			return fmt.Errorf("couldn't close the running copy of Rust: %w", err)
		}
		waitForProcessGone(rustProcess, 20*time.Second)
	}

	uri := SteamConnectURI(a)
	// "start" is the shell's own URL opener. cmd needs the empty "" title argument
	// before the URL, otherwise it treats the quoted URL as a window title.
	cmd := exec.Command("cmd", "/c", "start", "", uri)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("Windows refused to open the Steam link: %w", err)
	}

	w.mu.Lock()
	w.closing = false
	if w.exited == nil {
		w.exited = make(chan Exit, 1)
	}
	ch := w.exited
	if w.watch != nil {
		close(w.watch)
	}
	stop := make(chan struct{})
	w.watch = stop
	w.mu.Unlock()

	go w.watchProcess(ch, stop)
	return nil
}

// watchProcess waits for the game to appear, then reports when it disappears.
func (w *WindowsLauncher) watchProcess(ch chan Exit, stop chan struct{}) {
	deadline := time.Now().Add(3 * time.Minute) // Rust and EAC take a while to start
	appeared := false
	for {
		select {
		case <-stop:
			return
		case <-time.After(2 * time.Second):
		}
		running := processRunning(rustProcess)
		if running {
			appeared = true
			continue
		}
		if appeared {
			w.mu.Lock()
			expected := w.closing
			w.mu.Unlock()
			select {
			case ch <- Exit{Code: 1, Expected: expected}:
			default:
			}
			return
		}
		if time.Now().After(deadline) {
			select {
			case ch <- Exit{Code: -1}:
			default:
			}
			return
		}
	}
}

func (w *WindowsLauncher) Running() bool { return processRunning(rustProcess) }

func (w *WindowsLauncher) Exited() <-chan Exit {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.exited == nil {
		w.exited = make(chan Exit, 1)
	}
	return w.exited
}

// Close asks Rust to shut down. taskkill without /F sends the process a close
// request, which is the same thing that happens when the user clicks the X.
// /F is the last resort if it ignores us.
func (w *WindowsLauncher) Close() error {
	w.mu.Lock()
	w.closing = true
	w.mu.Unlock()

	if !processRunning(rustProcess) {
		return nil
	}
	_ = exec.Command("taskkill", "/IM", rustProcess).Run()
	if waitForProcessGone(rustProcess, 15*time.Second) {
		return nil
	}
	return exec.Command("taskkill", "/F", "/IM", rustProcess).Run()
}

// processRunning shells out to tasklist. Boring, dependency-free, and it needs
// no special privileges.
func processRunning(name string) bool {
	out, err := exec.Command("tasklist", "/FI", "IMAGENAME eq "+name, "/NH").Output()
	if err != nil {
		return false
	}
	return strings.Contains(strings.ToLower(string(out)), strings.ToLower(name))
}

func waitForProcessGone(name string, within time.Duration) bool {
	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		if !processRunning(name) {
			return true
		}
		time.Sleep(500 * time.Millisecond)
	}
	return false
}
