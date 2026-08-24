//go:build !windows

package game

// Steam's install layout is only inspected on Windows, where the game runs.
func RustUpdateState() UpdateState { return UpdateState{} }
