package support

import (
	_ "embed"
)

// The troubleshooting guide is compiled into the relay so an answer never
// depends on a file being present on the machine. The canonical copy is
// docs/troubleshooting.md, where the humans edit it; scripts/sync-embedded.sh
// copies it here and a test fails loudly if the two drift apart.
//
//go:embed troubleshooting.md
var troubleshooting string

// Troubleshooting returns the guide every answer is built from.
func Troubleshooting() string { return troubleshooting }
