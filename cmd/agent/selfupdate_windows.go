//go:build windows

package main

import (
	"log/slog"
	"os"
	"os/exec"
	"time"

	"queueup/internal/update"
)

// The agent keeps itself current. Nobody should be quitting tray icons and
// replacing files by hand, least of all mid-wipe: the whole product is "your
// PC handles it while you are away", and that has to include the agent itself.
//
// Rules it lives by:
//   - never swap while a join is running; wipe day is exactly when an update
//     lands and exactly when interrupting would hurt most
//   - never touch a development build, never downgrade (update.ShouldUpdate)
//   - a failed update changes nothing; the running version carries on
const (
	updateCheckEvery = 6 * time.Hour
	updateRetryEvery = 10 * time.Minute // when a check found one but a job was running
)

// startSelfUpdater begins the periodic check. Called once, from the tray.
func startSelfUpdater() {
	go func() {
		// A moment after startup, then on a slow clock.
		timer := time.NewTimer(2 * time.Minute)
		defer timer.Stop()
		for range timer.C {
			timer.Reset(selfUpdateOnce())
		}
	}()
}

// selfUpdateOnce does one round and says when to try again.
func selfUpdateOnce() time.Duration {
	rel, err := update.Check(nil)
	if err != nil {
		slog.Warn("update check failed; will try again later", "err", err)
		return updateCheckEvery
	}
	if !update.ShouldUpdate(Version, rel.Version) {
		return updateCheckEvery
	}

	// There is an update. Not while a join is running.
	if busyCheck != nil && busyCheck() {
		slog.Info("update available but a join is running; deferring", "version", rel.Version)
		return updateRetryEvery
	}

	exe, err := os.Executable()
	if err != nil {
		return updateCheckEvery
	}
	slog.Info("updating the agent", "from", Version, "to", rel.Version)

	next, err := update.Download(nil, rel, os.TempDir())
	if err != nil {
		slog.Warn("update download failed; staying on this version", "err", err)
		return updateCheckEvery
	}

	// Last look before the swap: a job may have started during the download.
	if busyCheck != nil && busyCheck() {
		_ = os.Remove(next)
		return updateRetryEvery
	}

	if err := update.Apply(exe, next); err != nil {
		slog.Warn("update failed and was rolled back; staying on this version", "err", err)
		return updateCheckEvery
	}

	// Hand over to the new version and bow out.
	if err := hidden(exec.Command(exe, "tray")).Start(); err != nil {
		slog.Error("the new version did not start; it will start at next login", "err", err)
	}
	os.Exit(0)
	return updateCheckEvery // unreachable
}
