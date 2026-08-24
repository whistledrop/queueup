// Package a2s asks a game server directly how it is doing, using Valve's
// standard query protocol (the same one the Steam server browser uses).
//
// This is what makes wipe-restart detection fast. It needs no API, no key and
// no third party: one small UDP packet to the server itself, answered in
// milliseconds. Polling one server every couple of seconds this way is less
// traffic than a single web page load and is exactly what the protocol is for.
//
// Rust reports its queue length inside the response's keywords field, as
// comma-separated tags like "mp200,cp199,qp312": mp is max players, cp current
// players, qp queued players. That qp number is the one a wipe-day tool wants.
package a2s

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"
)

// Info is what the server said about itself.
type Info struct {
	Name       string
	Map        string
	Players    int
	MaxPlayers int
	Queue      int // from the qp keyword; 0 if the server does not report one
	Keywords   string
}

var queryPayload = append([]byte{0xFF, 0xFF, 0xFF, 0xFF, 'T'},
	append([]byte("Source Engine Query"), 0x00)...)

// Query asks one server for its info. A server that is down, restarting for a
// wipe, or unreachable simply times out; the caller treats any error as "down".
func Query(ctx context.Context, addr string, timeout time.Duration) (Info, error) {
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	d := net.Dialer{Timeout: timeout}
	conn, err := d.DialContext(ctx, "udp", addr)
	if err != nil {
		return Info{}, fmt.Errorf("reaching %s: %w", addr, err)
	}
	defer conn.Close()

	deadline := time.Now().Add(timeout)
	if dl, ok := ctx.Deadline(); ok && dl.Before(deadline) {
		deadline = dl
	}
	_ = conn.SetDeadline(deadline)

	resp, err := roundTrip(conn, queryPayload)
	if err != nil {
		return Info{}, err
	}

	// The server may demand a challenge first (an anti-spoofing measure): a
	// 0x41 reply carrying 4 bytes to echo back appended to the original query.
	if len(resp) >= 5 && resp[4] == 0x41 {
		if len(resp) < 9 {
			return Info{}, fmt.Errorf("%s sent a malformed challenge", addr)
		}
		challenged := append(append([]byte{}, queryPayload...), resp[5:9]...)
		resp, err = roundTrip(conn, challenged)
		if err != nil {
			return Info{}, err
		}
	}

	return parseInfo(resp, addr)
}

func roundTrip(conn net.Conn, payload []byte) ([]byte, error) {
	if _, err := conn.Write(payload); err != nil {
		return nil, err
	}
	buf := make([]byte, 4096)
	n, err := conn.Read(buf)
	if err != nil {
		return nil, err
	}
	return buf[:n], nil
}

// parseInfo reads an A2S_INFO (0x49) response.
func parseInfo(resp []byte, addr string) (Info, error) {
	if len(resp) < 5 || !bytes.HasPrefix(resp, []byte{0xFF, 0xFF, 0xFF, 0xFF}) || resp[4] != 0x49 {
		return Info{}, fmt.Errorf("%s sent something that isn't a server info reply", addr)
	}
	r := &reader{buf: resp[5:]}

	r.byte() // protocol version, unused
	info := Info{Name: r.cstring(), Map: r.cstring()}
	r.cstring() // folder
	r.cstring() // game
	r.uint16()  // steam app id
	info.Players = int(r.byte())
	info.MaxPlayers = int(r.byte())
	r.byte() // bots
	r.byte() // server type
	r.byte() // environment
	r.byte() // visibility
	r.byte() // vac

	r.cstring() // version
	if r.failed {
		return Info{}, fmt.Errorf("%s sent a truncated info reply", addr)
	}

	// Extra data, present when the flags byte says so. Keywords carry Rust's
	// queue length.
	if edf := r.byte(); !r.failed {
		if edf&0x80 != 0 {
			r.uint16() // game port
		}
		if edf&0x10 != 0 {
			r.skip(8) // server steam id
		}
		if edf&0x40 != 0 {
			r.uint16()  // spectator port
			r.cstring() // spectator name
		}
		if edf&0x20 != 0 {
			info.Keywords = r.cstring()
			info.Queue = QueueFromKeywords(info.Keywords)
		}
	}
	return info, nil
}

// QueueFromKeywords digs the queued-player count out of Rust's keyword tags.
// Steam's server list carries the same tags in its "gametype" field, so the
// search results can show a queue length without querying anything.
func QueueFromKeywords(keywords string) int {
	for _, tag := range strings.Split(keywords, ",") {
		if rest, ok := strings.CutPrefix(strings.TrimSpace(tag), "qp"); ok {
			if n, err := strconv.Atoi(rest); err == nil && n >= 0 {
				return n
			}
		}
	}
	return 0
}

// reader walks the response without panicking on short input.
type reader struct {
	buf    []byte
	failed bool
}

func (r *reader) byte() byte {
	if len(r.buf) < 1 {
		r.failed = true
		return 0
	}
	b := r.buf[0]
	r.buf = r.buf[1:]
	return b
}

func (r *reader) uint16() uint16 {
	if len(r.buf) < 2 {
		r.failed = true
		return 0
	}
	v := binary.LittleEndian.Uint16(r.buf)
	r.buf = r.buf[2:]
	return v
}

func (r *reader) skip(n int) {
	if len(r.buf) < n {
		r.failed = true
		r.buf = nil
		return
	}
	r.buf = r.buf[n:]
}

func (r *reader) cstring() string {
	i := bytes.IndexByte(r.buf, 0)
	if i < 0 {
		r.failed = true
		return ""
	}
	s := string(r.buf[:i])
	r.buf = r.buf[i+1:]
	return s
}
