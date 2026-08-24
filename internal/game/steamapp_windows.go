//go:build windows

package game

import (
	"os"
	"path/filepath"
	"regexp"

	"golang.org/x/sys/windows/registry"
)

// RustUpdateState asks Steam what it is doing with Rust on this machine.
//
// Steam records this in steamapps\appmanifest_252490.acf, next to wherever the
// game is installed. Games can live on any drive, so the library folders are
// looked up rather than assumed.
func RustUpdateState() UpdateState {
	for _, dir := range steamLibraries() {
		manifest := filepath.Join(dir, "steamapps", "appmanifest_"+RustAppID+".acf")
		if st, err := readAppManifest(manifest); err == nil && st.Known {
			return st
		}
	}
	return UpdateState{}
}

// steamPath finds Steam itself, from the registry, falling back to the usual
// install locations.
func steamPath() string {
	for _, root := range []registry.Key{registry.CURRENT_USER, registry.LOCAL_MACHINE} {
		for _, path := range []string{`Software\Valve\Steam`, `Software\WOW6432Node\Valve\Steam`} {
			key, err := registry.OpenKey(root, path, registry.QUERY_VALUE)
			if err != nil {
				continue
			}
			for _, name := range []string{"SteamPath", "InstallPath"} {
				v, _, err := key.GetStringValue(name)
				if err == nil && v != "" {
					key.Close()
					return filepath.Clean(v)
				}
			}
			key.Close()
		}
	}
	for _, guess := range []string{
		`C:\Program Files (x86)\Steam`,
		`C:\Program Files\Steam`,
	} {
		if _, err := os.Stat(guess); err == nil {
			return guess
		}
	}
	return ""
}

var libraryPath = regexp.MustCompile(`"path"\s+"([^"]+)"`)

// steamLibraries lists every folder Steam installs games into, because Rust is
// very often on a different drive from Steam itself.
func steamLibraries() []string {
	steam := steamPath()
	if steam == "" {
		return nil
	}
	dirs := []string{steam}

	raw, err := os.ReadFile(filepath.Join(steam, "steamapps", "libraryfolders.vdf"))
	if err != nil {
		return dirs
	}
	for _, m := range libraryPath.FindAllStringSubmatch(string(raw), -1) {
		// The VDF escapes backslashes.
		dir := filepath.Clean(unescapeVDF(m[1]))
		if dir != "" && dir != steam {
			dirs = append(dirs, dir)
		}
	}
	return dirs
}

func unescapeVDF(s string) string {
	out := make([]rune, 0, len(s))
	prevSlash := false
	for _, r := range s {
		if r == '\\' && !prevSlash {
			prevSlash = true
			continue
		}
		out = append(out, r)
		prevSlash = false
	}
	return string(out)
}
