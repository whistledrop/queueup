//go:build windows

package game

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

// createNoWindow is the Windows flag for "start this child without giving it a
// console". The agent itself is built windowless, so without this every helper
// it runs gets a brand new console window of its own. Since the running check
// happens every couple of seconds, that means a black window blinking on the
// player's screen forever. Anything we spawn must be silent.
const createNoWindow = 0x08000000

// silent prepares a command to run without flashing a window.
func silent(cmd *exec.Cmd) *exec.Cmd {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
	return cmd
}

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

	mu            sync.Mutex
	addr          Addr // what the current launch was aimed at, for a re-send
	exited        chan Exit
	watch         chan struct{}
	closing       bool
	update        UpdateState
	stall         *stallWatch
	updateChecked time.Time
}

// DefaultLogPath is where the Rust client writes its log on this machine:
// the game's own install folder first (that is where -logfile points), the
// legacy AppData location as a fallback.
func DefaultLogPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = ""
	}
	return FindRustLog(home, RustInstallDirs())
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
	// Steam not running is NOT a blocker, and refusing on it was wrong.
	//
	// Opening a steam:// link is how a person starts Rust from a web page, and
	// Windows starts Steam to handle it, signing in on its own for anybody who
	// ticked "remember me", which is nearly everybody. Refusing here meant
	// never handing the link over, so we failed at the one thing that would
	// have worked.
	//
	// It matters most on the day this product exists for. A Rust patch usually
	// brings a Steam client update with it, Steam restarts itself to apply
	// that, and a PC that rebooted overnight may not have Steam up at all. In
	// every one of those cases the right move is to open the link and wait,
	// which is what the launch watcher does, patiently, and with better words
	// if it really never arrives.
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

	if err := openSteamURI(a); err != nil {
		return err
	}

	w.mu.Lock()
	w.addr = a
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

// openSteamURI hands the launch link to Windows, exactly as if the user had
// clicked a steam:// link in a browser.
func openSteamURI(a Addr) error {
	uri := SteamConnectURI(a)
	// "start" is the shell's own URL opener. cmd needs the empty "" title argument
	// before the URL, otherwise it treats the quoted URL as a window title.
	if err := silent(exec.Command("cmd", "/c", "start", "", uri)).Run(); err != nil {
		return fmt.Errorf("Windows refused to open the Steam link: %w", err)
	}
	return nil
}

// watchProcess waits for the game to appear, then reports when it disappears.
func (w *WindowsLauncher) watchProcess(ch chan Exit, stop chan struct{}) {
	// Rust and Easy Anti-Cheat take a while to start, hence the grace period
	// before we conclude the game is never coming.
	const startupGrace = 3 * time.Minute
	deadline := time.Now().Add(startupGrace)
	appeared := false
	steamSeen := false
	nudges := 0

	for {
		select {
		case <-stop:
			return
		case <-time.After(2 * time.Second):
		}
		running := processRunning(rustProcess)
		if running {
			appeared = true
			w.setUpdate(UpdateState{})
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

		// The game has not started yet. On force wipe that is expected and can
		// last a long time: Rust ships an update with the wipe, and Steam must
		// download several gigabytes first. Giving up here would fail on the one
		// day this product exists for, so while Steam is genuinely working, we
		// wait, and keep the phone informed.
		update := w.freshUpdate()
		w.setUpdate(update)

		// Steam arriving is progress worth paying for. It may have been off,
		// or restarting to apply its own update, and either way the clock on
		// the game appearing should start from when Steam was actually ready
		// rather than from when we asked.
		steamUp := processRunning(steamProcess)
		if steamUp && !steamSeen {
			steamSeen = true
			deadline = time.Now().Add(startupGrace)
		}

		// Steam knows an update is needed but has not started it. Ask it to
		// launch the game again: that is what makes Steam deal with the update.
		if shouldNudgeLaunch(update, appeared, nudges) {
			nudges++
			w.mu.Lock()
			a := w.addr
			w.mu.Unlock()
			if err := openSteamURI(a); err == nil {
				deadline = time.Now().Add(startupGrace)
			}
		}

		switch judgeLaunchWait(update, steamUp, time.Now().After(deadline)) {
		case verdictExtendGrace:
			deadline = time.Now().Add(startupGrace)
		case verdictKeepWaiting:
			// within patience; check again shortly
		case verdictGiveUp, verdictGiveUpBlaming:
			// A wedged download is worth explaining. Waiting longer will not fix
			// a paused Steam or a full disk, and the player needs to know that.
			ex := Exit{Code: -1, Reason: launchFailureReason(update, appeared, steamSeen)}
			select {
			case ch <- ex:
			default:
			}
			return
		}
	}
}

// UpdateProgress reports whether Steam is currently updating the game, for the
// agent to relay to the player. It is an optional part of the Launcher
// contract, discovered by type assertion.
//
// It also answers while a job is only WAITING for a wipe restart, so a pending
// update can be flagged before the server even comes back, giving the player a
// chance to start the download early rather than discovering it at the worst
// moment.
func (w *WindowsLauncher) UpdateProgress() UpdateState {
	w.mu.Lock()
	cached, checked := w.update, w.updateChecked
	w.mu.Unlock()

	// Reading a small file is cheap, but not every tick.
	if time.Since(checked) < 5*time.Second {
		return cached
	}
	if processRunning(rustProcess) {
		// Already running, so nothing is pending by definition.
		w.setUpdate(UpdateState{})
		return UpdateState{}
	}
	fresh := w.freshUpdate()
	w.setUpdate(fresh)
	return fresh
}

// freshUpdate reads Steam's manifest and folds in how long the download has
// been sitting still.
func (w *WindowsLauncher) freshUpdate() UpdateState {
	u := RustUpdateState()
	if _, faked := FakeUpdate(); faked {
		// The pretend state already says what it means, StalledFor included.
		// Running it through the stall watch would zero that out, because a
		// pretend byte count never moves and the watch would call it fresh.
		return u
	}
	w.mu.Lock()
	if w.stall == nil {
		w.stall = newStallWatch(time.Now)
	}
	watch := w.stall
	w.mu.Unlock()
	u.StalledFor = watch.stalledFor(u)
	return u
}

func (w *WindowsLauncher) setUpdate(u UpdateState) {
	w.mu.Lock()
	w.update = u
	w.updateChecked = time.Now()
	w.mu.Unlock()
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
	_ = silent(exec.Command("taskkill", "/IM", rustProcess)).Run()
	if waitForProcessGone(rustProcess, 15*time.Second) {
		return nil
	}
	return silent(exec.Command("taskkill", "/F", "/IM", rustProcess)).Run()
}

// processRunning shells out to tasklist. Boring, dependency-free, and it needs
// no special privileges.
func processRunning(name string) bool {
	out, err := silent(exec.Command("tasklist", "/FI", "IMAGENAME eq "+name, "/NH")).Output()
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
