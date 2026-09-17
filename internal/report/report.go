// Package report bundles everything needed to diagnose a problem into one file
// a player can send.
//
// The people testing this are Rust players, not engineers, and the person who
// can read the logs is in another country. "Right-click the icon, save a
// report, send me the file" is an instruction anyone can follow at midnight on
// wipe day. "Open PowerShell and find your Steam library" is not.
package report

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Inputs is everything worth bundling.
type Inputs struct {
	AgentVersion string
	AgentLogPath string // the agent's own log
	GameLogPath  string // Rust's log, wherever the agent found it
	Now          func() time.Time
}

// capBytes bounds how much of each log goes in. Logs grow; the useful part is
// the recent end, and a report has to stay small enough to send over chat.
const capBytes = 512 * 1024

// Build writes the report into dir and returns its path.
func Build(dir string, in Inputs) (string, error) {
	name, content := Render(in)
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, content, 0o644); err != nil {
		return "", fmt.Errorf("saving the report: %w", err)
	}
	return path, nil
}

// Render produces the report without saving it, for sending straight to
// QueueUp. It returns a suggested file name and the contents.
func Render(in Inputs) (name string, content []byte) {
	now := time.Now
	if in.Now != nil {
		now = in.Now
	}
	stamp := now().UTC()

	var b strings.Builder
	fmt.Fprintf(&b, "QueueUp problem report\n")
	fmt.Fprintf(&b, "made:    %s\n", stamp.Format(time.RFC3339))
	fmt.Fprintf(&b, "version: %s\n", in.AgentVersion)
	fmt.Fprintf(&b, "\nThis contains the QueueUp agent's log and the most recent part of the\n")
	fmt.Fprintf(&b, "game's own log, nothing else. No passwords. Rust writes your Steam ID\n")
	fmt.Fprintf(&b, "into its log, and file locations can include your Windows user name,\n")
	fmt.Fprintf(&b, "so this report can contain both.\n")

	appendSection(&b, "AGENT LOG ("+in.AgentLogPath+")", in.AgentLogPath)
	appendSection(&b, "GAME LOG ("+in.GameLogPath+")", in.GameLogPath)

	name = "QueueUp-report-" + stamp.Format("2006-01-02-150405") + ".txt"
	return name, []byte(b.String())
}

// appendSection adds the tail of one file, or says plainly why it could not.
func appendSection(b *strings.Builder, title, path string) {
	fmt.Fprintf(b, "\n\n===== %s =====\n", title)
	if path == "" {
		fmt.Fprint(b, "(no path known for this log)\n")
		return
	}
	tail, size, err := readTail(path, capBytes)
	if err != nil {
		fmt.Fprintf(b, "(could not read it: %v)\n", err)
		return
	}
	if size > capBytes {
		fmt.Fprintf(b, "(the file is %d bytes; this is the last %d)\n", size, capBytes)
	}
	b.Write(tail)
	if len(tail) == 0 || tail[len(tail)-1] != '\n' {
		b.WriteByte('\n')
	}
}

// readTail returns up to max bytes from the end of the file, and its full size.
func readTail(path string, max int64) ([]byte, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil, 0, err
	}
	start := int64(0)
	if st.Size() > max {
		start = st.Size() - max
	}
	if _, err := f.Seek(start, io.SeekStart); err != nil {
		return nil, 0, err
	}
	out, err := io.ReadAll(f)
	return out, st.Size(), err
}
