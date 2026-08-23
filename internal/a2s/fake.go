package a2s

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"net"
	"sync"
)

// FakeServer pretends to be a Rust server answering queries. Tests and the
// simulator point the poller at one of these, so wipe-restart detection is
// exercised without any real server involved.
type FakeServer struct {
	mu        sync.Mutex
	conn      *net.UDPConn
	info      Info
	online    bool
	challenge bool // demand the challenge handshake, as many real servers do
}

// NewFakeServer starts one on a random local port.
func NewFakeServer(initial Info, online bool) (*FakeServer, error) {
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		return nil, err
	}
	f := &FakeServer{conn: conn, info: initial, online: online, challenge: true}
	go f.serve()
	return f, nil
}

// Addr is where to point the poller.
func (f *FakeServer) Addr() string { return f.conn.LocalAddr().String() }

// Close shuts the fake down.
func (f *FakeServer) Close() { _ = f.conn.Close() }

// SetOnline flips the server up or down. Down means queries go unanswered,
// exactly like a server that is restarting for a wipe.
func (f *FakeServer) SetOnline(online bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.online = online
}

// SetInfo updates what the fake reports.
func (f *FakeServer) SetInfo(info Info) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.info = info
}

func (f *FakeServer) serve() {
	buf := make([]byte, 2048)
	var pending []byte // the challenge we handed out
	for {
		n, remote, err := f.conn.ReadFromUDP(buf)
		if err != nil {
			return
		}
		f.mu.Lock()
		online, info, challenge := f.online, f.info, f.challenge
		f.mu.Unlock()

		if !online || !bytes.HasPrefix(buf[:n], []byte{0xFF, 0xFF, 0xFF, 0xFF}) {
			continue // a down server just says nothing
		}

		if challenge && !bytes.HasSuffix(buf[:n], pending) {
			pending = []byte{0xDE, 0xAD, 0xBE, 0xEF}
			out := append([]byte{0xFF, 0xFF, 0xFF, 0xFF, 0x41}, pending...)
			_, _ = f.conn.WriteToUDP(out, remote)
			continue
		}
		_, _ = f.conn.WriteToUDP(encodeInfo(info), remote)
	}
}

// encodeInfo builds a realistic A2S_INFO reply, keywords included.
func encodeInfo(info Info) []byte {
	var b bytes.Buffer
	b.Write([]byte{0xFF, 0xFF, 0xFF, 0xFF, 0x49})
	b.WriteByte(17) // protocol
	writeC := func(s string) { b.WriteString(s); b.WriteByte(0) }
	writeC(info.Name)
	writeC(info.Map)
	writeC("rust")
	writeC("Rust")
	appID := make([]byte, 2)
	binary.LittleEndian.PutUint16(appID, 252490%0x10000)
	b.Write(appID)
	b.WriteByte(byte(info.Players))
	b.WriteByte(byte(info.MaxPlayers))
	b.WriteByte(0)   // bots
	b.WriteByte('d') // dedicated
	b.WriteByte('l') // linux
	b.WriteByte(0)   // public
	b.WriteByte(1)   // vac
	writeC("2511")   // version

	keywords := info.Keywords
	if keywords == "" {
		keywords = fmt.Sprintf("mp%d,cp%d,qp%d", info.MaxPlayers, info.Players, info.Queue)
	}
	b.WriteByte(0x20) // EDF: keywords only
	writeC(keywords)
	return b.Bytes()
}
