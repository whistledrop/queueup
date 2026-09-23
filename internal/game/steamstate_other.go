//go:build !windows

package game

// RustManifestPath is Windows-only; Steam's layout is not inspected elsewhere.
func RustManifestPath() string { return "" }
