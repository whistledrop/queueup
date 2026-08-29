// Package agentcfg stores the agent's settings on the player's PC: which relay
// to talk to, and the token that identifies this machine.
//
// The file contains no Steam credentials, because the agent never has any. It
// holds a token that this PC uses to identify itself to the relay, and nothing
// else. If the file is deleted the agent simply asks to be paired again.
package agentcfg

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Config is what lives in agent.json.
type Config struct {
	RelayURL string `json:"relay_url"`
	// WebURL is where the QueueUp website lives, for the tray's "open the
	// website" item. Optional; defaults to the relay address.
	WebURL      string `json:"web_url,omitempty"`
	DeviceToken string `json:"device_token,omitempty"`
	DeviceID    string `json:"device_id,omitempty"`
	DeviceName  string `json:"device_name,omitempty"`

	// AutostartOff records that the user deliberately turned off starting with
	// Windows. Without it, pairing would helpfully switch it back on every time
	// and quietly overrule them.
	//
	// Absent means "never said", which is treated as yes: a PC that does not
	// come back after a restart cannot do the one job this product has.
	AutostartOff bool `json:"autostart_off,omitempty"`
}

// Paired reports whether this PC has been linked to an account.
func (c Config) Paired() bool { return c.DeviceToken != "" }

// DefaultPath is where the settings live: %AppData%\QueueUp\agent.json on
// Windows, and the equivalent per-user config folder elsewhere.
func DefaultPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("finding your settings folder: %w", err)
	}
	return filepath.Join(dir, "QueueUp", "agent.json"), nil
}

// Load reads the settings. A missing file is not an error: it means this PC has
// not been paired yet.
func Load(path string) (Config, error) {
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return Config{}, nil
	}
	if err != nil {
		return Config{}, fmt.Errorf("reading %s: %w", path, err)
	}
	var c Config
	if err := json.Unmarshal(raw, &c); err != nil {
		return Config{}, fmt.Errorf("%s is corrupted. Delete it and pair this PC again: %w", path, err)
	}
	return c, nil
}

// Save writes the settings, creating the folder if needed. The file is written
// so that only the current user can read it, because it holds this PC's token.
func Save(path string, c Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("creating %s: %w", filepath.Dir(path), err)
	}
	raw, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, append(raw, '\n'), 0o600); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	return nil
}
