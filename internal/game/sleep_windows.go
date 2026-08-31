//go:build windows

package game

import "os/exec"

// SleepTimeoutMinutes asks Windows how long until this PC puts itself to sleep
// while plugged in. SleepNever (0) means it stays awake; SleepUnknown (-1)
// means we could not tell, and nothing should be claimed on that basis.
func SleepTimeoutMinutes() int {
	out, err := silent(exec.Command("powercfg", "/q", "SCHEME_CURRENT", "SUB_SLEEP", "STANDBYIDLE")).Output()
	if err != nil {
		return SleepUnknown
	}
	return parseSleepTimeout(string(out))
}
