// Package logtail follows a growing log file, like "tail -f".
//
// It has to cope with three things the Rust client actually does: the file may
// not exist yet when we start watching, it gets truncated every time the game
// launches, and it can be replaced outright. Polling with os.Stat handles all
// three and works identically on Windows and macOS, which matters because all
// development happens on a Mac.
package logtail

import (
	"bufio"
	"context"
	"io"
	"os"
	"strings"
	"time"
)

// Follow watches path and calls onLine for every complete line appended to it.
// It returns when ctx is cancelled. It never returns an error: a missing or
// unreadable file is a normal, temporary condition here.
//
// fromStart controls what happens with content that is ALREADY in the file when
// we begin watching. The agent passes false: at that point the file still holds
// the previous Rust session, and reading it would make the agent think it had
// just been banned, or had just joined, based on something that happened
// yesterday.
//
// This only applies to the first open. Once the game launches it truncates the
// log, and a truncated or replaced file is always read from the top, because
// everything in it then belongs to the session we started.
func Follow(ctx context.Context, path string, poll time.Duration, fromStart bool, onLine func(string)) {
	if poll <= 0 {
		poll = 250 * time.Millisecond
	}

	var (
		f       *os.File
		reader  *bufio.Reader
		info    os.FileInfo
		partial strings.Builder
		opened  bool // after the first open, always read from the top
	)

	// Snapshot the file as it is right now. Anything already in it belongs to a
	// previous Rust session, and with fromStart=false we skip exactly that much
	// and no more. If the file does not exist yet, or is replaced before we get
	// to open it, there is nothing stale to skip and we read from the top.
	skipInfo, skipErr := os.Stat(path)
	skipSize := int64(0)
	if !fromStart && skipErr == nil {
		skipSize = skipInfo.Size()
	}
	closeFile := func() {
		if f != nil {
			_ = f.Close()
			f, reader, info = nil, nil, nil
		}
		partial.Reset()
	}
	defer closeFile()

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		st, err := os.Stat(path)
		switch {
		case err != nil:
			// Not there yet, or gone. Wait for it to show up.
			closeFile()
		case f == nil:
			nf, oerr := os.Open(path)
			if oerr == nil {
				f, reader, info = nf, bufio.NewReader(nf), st
				sameAsBefore := skipErr == nil && os.SameFile(st, skipInfo) && st.Size() >= skipSize
				if !fromStart && !opened && sameAsBefore && skipSize > 0 {
					if _, serr := f.Seek(skipSize, io.SeekStart); serr == nil {
						reader.Reset(f)
					}
				}
				opened = true
			}
		case st.Size() < info.Size() || !os.SameFile(st, info):
			// Truncated (the game relaunched) or replaced. Start over from the top.
			closeFile()
			continue
		default:
			info = st
		}

		if reader != nil {
			for {
				chunk, rerr := reader.ReadString('\n')
				if chunk != "" {
					partial.WriteString(chunk)
				}
				if rerr != nil {
					break // EOF for now; the rest arrives on a later poll
				}
				line := strings.TrimRight(partial.String(), "\r\n")
				partial.Reset()
				if line != "" {
					onLine(line)
				}
			}
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(poll):
		}
	}
}
