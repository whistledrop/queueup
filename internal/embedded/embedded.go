// Package embedded compiles the agent's data files into the executable, so
// QueueUpAgent.exe really is one file: no folders to copy alongside it, no
// "works in the project folder but not on the PC" failures.
//
// The log patterns keep their patch-day escape hatch: a patterns.json placed
// next to the agent's settings file overrides the built-in copy, so a Rust
// update is still fixed by dropping in one file, with no rebuild.
package embedded

import (
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strings"
)

//go:embed patterns.json
var patternsFS embed.FS

//go:embed scenarios/*.json
var scenariosFS embed.FS

// Patterns returns the built-in log pattern file.
func Patterns() []byte {
	b, err := patternsFS.ReadFile("patterns.json")
	if err != nil {
		panic("the embedded pattern file is missing: " + err.Error())
	}
	return b
}

// Scenario returns a built-in scenario by bare name ("long_queue").
func Scenario(name string) ([]byte, error) {
	b, err := scenariosFS.ReadFile("scenarios/" + name + ".json")
	if err != nil {
		return nil, fmt.Errorf("no built-in scenario called %q. Built in: %s",
			name, strings.Join(ScenarioNames(), ", "))
	}
	return b, nil
}

// ScenarioNames lists what is built in, for error messages and --help.
func ScenarioNames() []string {
	entries, err := fs.ReadDir(scenariosFS, "scenarios")
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		out = append(out, strings.TrimSuffix(e.Name(), ".json"))
	}
	sort.Strings(out)
	return out
}
