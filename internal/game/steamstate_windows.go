//go:build windows

package game

import "path/filepath"

// RustManifestPath returns the Steam file QueueUp reads to see what Steam is
// doing with Rust, or "" when there isn't one. Used by the steam-state command
// so a person can look at the same file we did.
func RustManifestPath() string {
	for _, dir := range steamLibraries() {
		manifest := filepath.Join(dir, "steamapps", "appmanifest_"+RustAppID+".acf")
		if st, err := readAppManifest(manifest); err == nil && st.Known {
			return manifest
		}
	}
	return ""
}
