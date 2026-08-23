package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"queueup/internal/agentcfg"
	"queueup/internal/embedded"
	"queueup/internal/logparse"
	"queueup/internal/scenario"
)

// loadPatterns finds the log pattern file, in this order:
//
//  1. an explicit --patterns path,
//  2. a patterns.json next to the agent's settings (the patch-day fix: when a
//     Rust update changes the log wording, drop an updated file there and
//     restart the agent; no rebuild, no reinstall),
//  3. the project checkout's configs/patterns.json, for development,
//  4. the copy built into the executable.
func loadPatterns(explicit string) (*logparse.Parser, string, error) {
	try := func(path string) (*logparse.Parser, string, error) {
		p, err := logparse.Load(path)
		if err != nil {
			return nil, "", fmt.Errorf("reading %s: %w", path, err)
		}
		return p, path, nil
	}

	if explicit != "" {
		return try(explicit)
	}
	if cfgPath, err := agentcfg.DefaultPath(); err == nil {
		override := filepath.Join(filepath.Dir(cfgPath), "patterns.json")
		if _, err := os.Stat(override); err == nil {
			return try(override)
		}
	}
	if _, err := os.Stat("configs/patterns.json"); err == nil {
		return try("configs/patterns.json")
	}

	p, err := logparse.Parse(embedded.Patterns())
	if err != nil {
		return nil, "", fmt.Errorf("the built-in pattern file is broken: %w", err)
	}
	return p, "built in", nil
}

// loadScenario accepts either a file path or the bare name of a built-in
// scenario, so the PC needs no testdata folder:
//
//	--scenario long_queue
//	--scenario testdata/scenarios/long_queue.json
func loadScenario(nameOrPath string) (*scenario.Scenario, error) {
	if _, err := os.Stat(nameOrPath); err == nil {
		return scenario.Load(nameOrPath)
	}
	name := strings.TrimSuffix(filepath.Base(nameOrPath), ".json")
	raw, err := embedded.Scenario(name)
	if err != nil {
		return nil, err
	}
	return scenario.Parse(raw)
}
