//go:build !windows

package game

// Steam's install layout is only inspected on Windows, where the game runs.
// The pretend state in RustUpdateState still works here, which is what lets the
// force-wipe messaging be tested away from a Windows machine.
func rustUpdateStateFromSteam() UpdateState { return UpdateState{} }
