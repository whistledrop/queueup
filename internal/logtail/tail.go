// Package logtail follows a growing log file, like "tail -f".
//
// It has to cope with three things the Rust client actually does: the file may
// not exist yet when we start watching, it gets emptied and started again every
// time the game launches, and it can be replaced outright.
//
// A new session is recognised by the first bytes of the file changing, and by
// the file being shorter than where we had read to, both checked through the
// open file itself. Neither the file's name nor its size alone is enough. A
// relaunch can refill the file past its old length between two looks, so it
// never appears to shrink; and on Windows the size reported for a file another
// program is writing can lag behind the real one. Either way the tailer used to
// carry on from the old session's end, in the middle of the new one, and miss
// everything that mattered.
package logtail

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"os"
	"strings"
	"time"
)

// headSize is how much of the start of the file identifies a session. Rust's
// first line carries a timestamp to the millisecond, so two sessions never
// begin with the same bytes.
const headSize = 256

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
// This only applies to the first open. Once the game launches it starts the
// log again, and a new session is always read from the top, because everything
// in it then belongs to the session we started.
func Follow(ctx context.Context, path string, poll time.Duration, fromStart bool, onLine func(string)) {
	if poll <= 0 {
		poll = 250 * time.Millisecond
	}

	var (
		f       *os.File
		reader  *bufio.Reader
		partial strings.Builder
		pos     int64  // how far into the file we have read
		head    []byte // the file's first bytes when we started reading it
		opened  bool   // after the first open, always read from the top
	)

	// What was there when we began. Only that file's existing content is
	// skipped: one that appears later, or replaces it before we get to open it,
	// is all new session and is read from the top.
	before, beforeErr := os.Stat(path)

	closeFile := func() {
		if f != nil {
			_ = f.Close()
			f, reader = nil, nil
		}
		partial.Reset()
		pos, head = 0, nil
	}
	defer closeFile()

	// readHead returns up to n bytes from the very start of the open file.
	readHead := func(n int) []byte {
		b := make([]byte, n)
		got, _ := f.ReadAt(b, 0)
		return b[:got]
	}

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
				f, reader = nf, bufio.NewReader(nf)
				// Anything already here belongs to a previous Rust session, and
				// with fromStart=false it is skipped, on the first open only. The
				// size comes from the open file, not the name, so it is the real
				// one even while Rust is writing.
				if !fromStart && !opened && beforeErr == nil && os.SameFile(st, before) {
					if fst, serr := f.Stat(); serr == nil && fst.Size() > 0 {
						if _, serr := f.Seek(fst.Size(), io.SeekStart); serr == nil {
							reader.Reset(f)
							pos = fst.Size()
						}
					}
				}
				head = readHead(headSize)
				opened = true
			}
		default:
			if fst, serr := f.Stat(); serr != nil || !os.SameFile(st, fst) {
				// Replaced by a different file. Start over from the top.
				closeFile()
				continue
			} else if fst.Size() < pos || !bytes.Equal(readHead(len(head)), head) {
				// Emptied and begun again: a new session, read from the top.
				// The first bytes changing catches the relaunch that refilled the
				// file past where we had read to before we looked.
				if _, serr := f.Seek(0, io.SeekStart); serr != nil {
					closeFile()
					continue
				}
				reader.Reset(f)
				partial.Reset()
				pos = 0
				head = readHead(headSize)
			}
		}

		if reader != nil {
			for {
				chunk, rerr := reader.ReadString('\n')
				if chunk != "" {
					partial.WriteString(chunk)
					pos += int64(len(chunk))
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
			// A file that was empty, or barely begun, when we first looked has
			// more of its start now. Keep the fullest head we can, so the next
			// comparison is against what the session really begins with.
			if len(head) < headSize && int64(len(head)) < pos {
				head = readHead(headSize)
			}
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(poll):
		}
	}
}
