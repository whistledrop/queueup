package game

import (
	"regexp"
	"strconv"
	"strings"
)

// A PC that goes to sleep is the quietest way for this product to fail. The
// agent does not stop, it does not error, and nothing is logged: the machine
// simply stops existing until somebody moves the mouse. If that happens twenty
// minutes before wipe, the join never runs and the first anyone knows is when
// they sit down to an empty server.
//
// Telling people to turn sleep off in a help page reaches whoever reads help
// pages. Reading the actual setting and saying "your PC is set to sleep after
// 30 minutes, here is how to change it" reaches everybody, and only bothers the
// people it applies to.

// SleepNever is the timeout value meaning the PC never sleeps by itself.
const SleepNever = 0

// SleepUnknown means we could not read the setting. Never warn on this: an
// unreadable setting is not evidence of a problem.
const SleepUnknown = -1

// acIndex matches the plugged-in sleep timeout in powercfg's output. Desktops
// live on AC; the battery figure is irrelevant to a gaming PC that waits at
// home, and warning about it would be noise.
var acIndex = regexp.MustCompile(`(?i)Current AC Power Setting Index:\s*(0x[0-9a-f]+|\d+)`)

// parseSleepTimeout reads the sleep timeout, in MINUTES, out of the output of
//
//	powercfg /q SCHEME_CURRENT SUB_SLEEP STANDBYIDLE
//
// The value powercfg prints is seconds, usually in hex.
func parseSleepTimeout(out string) int {
	m := acIndex.FindStringSubmatch(out)
	if m == nil {
		return SleepUnknown
	}
	raw := strings.TrimSpace(m[1])
	var seconds int64
	var err error
	if after, ok := strings.CutPrefix(strings.ToLower(raw), "0x"); ok {
		seconds, err = strconv.ParseInt(after, 16, 64)
	} else {
		seconds, err = strconv.ParseInt(raw, 10, 64)
	}
	if err != nil || seconds < 0 {
		return SleepUnknown
	}
	if seconds == 0 {
		return SleepNever
	}
	// Round up, so 90 seconds reads as "1 minute" rather than "never".
	return int((seconds + 59) / 60)
}
