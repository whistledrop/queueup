//go:build !windows

package game

// The sleep setting is a Windows question, asked of the machine that waits at
// home. Everywhere else it is simply unknown.
func SleepTimeoutMinutes() int { return SleepUnknown }
