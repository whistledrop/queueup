package game

import "time"

// SteamRestartGrace is how long Steam is given to come back before we conclude
// it is genuinely not running.
//
// Steam restarts itself when it updates, and on force wipe day that is likely
// to happen at exactly the wrong moment. During that gap steam.exe is missing,
// which looks identical to the player never having started Steam at all. Since
// telling somebody a thousand miles away to "start Steam" when Steam is in the
// middle of restarting is both wrong and useless, we wait a little first.
const SteamRestartGrace = 90 * time.Second

// awaitProcess waits for running() to report true, up to within. It returns
// true as soon as the process appears, and false if it never does.
//
// The clock and the sleep are passed in so this can be tested without waiting
// ninety real seconds, and on machines that have no Steam at all.
func awaitProcess(running func() bool, within time.Duration, now func() time.Time, pause func(time.Duration)) bool {
	const poll = 2 * time.Second
	deadline := now().Add(within)
	for {
		if running() {
			return true
		}
		if !now().Before(deadline) {
			return false
		}
		pause(poll)
	}
}
