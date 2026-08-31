// Package relayclient is the agent's connection to the relay.
//
// It dials out and keeps one WebSocket open. Nothing ever connects in to the
// player's PC, so there is no port forwarding, no UPnP, and nothing to change on
// their router. If the connection drops, for any reason at all, it comes back on
// its own.
package relayclient

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/coder/websocket"

	"queueup/internal/protocol"
)

// Handler receives everything the relay tells the agent to do.
type Handler interface {
	OnConnected(protocol.Welcome)
	OnDisconnected(error)
	OnAssign(protocol.Job)
	OnCancel(protocol.Cancel)
	OnServerStatus(protocol.ServerStatus)
}

// Client keeps the connection to the relay alive.
type Client struct {
	RelayURL     string
	DeviceToken  string
	AgentVersion string
	OS           string
	Hostname     string
	Simulator    bool
	// SleepAfterMinutes is reported to the relay so the app can warn about a PC
	// that puts itself to sleep. 0 never, -1 unknown.
	SleepAfterMinutes int
	Handler           Handler
	Log               *slog.Logger

	// MaxBackoff caps how long we wait between reconnection attempts. Thirty
	// seconds: long enough not to hammer a relay that is down, short enough that
	// a PC is never sitting idle for minutes on wipe day.
	MaxBackoff time.Duration

	out     chan []byte
	capture func(t protocol.Type, payload any)
}

// SocketURL turns the relay's base address into the agent WebSocket address.
func SocketURL(base string) (string, error) {
	u, err := url.Parse(strings.TrimSuffix(base, "/"))
	if err != nil {
		return "", fmt.Errorf("%q is not a valid relay address: %w", base, err)
	}
	switch u.Scheme {
	case "https", "wss":
		u.Scheme = "wss"
	case "http", "ws":
		u.Scheme = "ws"
	default:
		return "", fmt.Errorf("%q should start with https:// or http://", base)
	}
	u.Path = strings.TrimSuffix(u.Path, "/") + "/agent"
	return u.String(), nil
}

// Backoff is how long to wait before reconnection attempt n (starting at 1).
// It doubles, caps, and adds a little randomness so that a relay coming back up
// does not get every agent in the world at the same instant.
func Backoff(attempt int, max time.Duration) time.Duration {
	if max <= 0 {
		max = 30 * time.Second
	}
	d := time.Second
	for i := 1; i < attempt && d < max; i++ {
		d *= 2
	}
	if d > max {
		d = max
	}
	jitter := int64(d / 4)
	if jitter <= 0 {
		return d
	}
	n, err := rand.Int(rand.Reader, big.NewInt(jitter))
	if err != nil {
		return d
	}
	return d + time.Duration(n.Int64())
}

// CaptureForTests routes every Send into fn instead of the socket, so the
// agent's behaviour can be asserted on without a relay. Test-only by
// convention; it has no effect once Run is under way with a real relay because
// nothing in production sets it.
func (c *Client) CaptureForTests(fn func(t protocol.Type, payload any)) {
	c.capture = fn
}

// Send queues a message for the relay. If we are not connected, the message is
// dropped rather than queued forever: the relay hands the job straight back on
// reconnection, and the agent reports its current state again, so nothing that
// matters is lost.
func (c *Client) Send(t protocol.Type, payload any) {
	if c.capture != nil {
		c.capture(t, payload)
		return
	}
	raw, err := protocol.Encode(t, payload)
	if err != nil {
		c.Log.Error("encoding message", "type", t, "err", err)
		return
	}
	select {
	case c.out <- raw:
	default:
	}
}

// Run connects and keeps reconnecting until ctx is cancelled. It only returns
// an error for problems that reconnecting cannot fix, such as this PC having
// been unlinked from the account.
func (c *Client) Run(ctx context.Context) error {
	if c.Log == nil {
		c.Log = slog.Default()
	}
	if c.MaxBackoff == 0 {
		c.MaxBackoff = 30 * time.Second
	}
	if c.out == nil {
		c.out = make(chan []byte, 64)
	}
	socket, err := SocketURL(c.RelayURL)
	if err != nil {
		return err
	}

	attempt := 0
	rejections := 0
	for {
		if ctx.Err() != nil {
			return nil
		}
		attempt++
		err := c.session(ctx, socket)

		if errors.Is(err, errRejected) {
			rejections++
			if rejections >= unlinkConfirmations {
				return fmt.Errorf("%w (refused %d times running)", ErrUnlinked, rejections)
			}
			c.Log.Warn("the relay refused this PC's token; asking again before believing it",
				"refusals", rejections, "of", unlinkConfirmations, "err", err)
		} else if err == nil {
			rejections = 0
		}

		switch {
		case ctx.Err() != nil:
			return nil
		case err != nil:
			wait := Backoff(attempt, c.MaxBackoff)
			c.Log.Warn("lost the connection to the relay, retrying",
				"in", wait.Round(time.Second), "attempt", attempt, "err", err)
			c.Handler.OnDisconnected(err)
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(wait):
			}
		default:
			attempt = 0
		}
	}
}

// ErrUnlinked means reconnecting will never help: this PC's token is no longer
// accepted, because the user unlinked it from their account. The caller should
// forget the token so the next start goes back to pairing.
var ErrUnlinked = errors.New("this PC has been unlinked from the account")

// errRejected is one refusal, which is not yet a conclusion. See
// unlinkConfirmations.
var errRejected = errors.New("the relay refused this PC's token")

// unlinkConfirmations is how many refusals in a row it takes before we believe
// the pairing is really gone.
//
// Concluding this is destructive and cannot be undone from here: the token is
// erased and somebody has to be at the PC to read a new pairing code. Plenty of
// things produce a passing 401 or 403 that have nothing to do with the pairing:
// a relay mid-deploy, a relay that cannot read its database, a captive portal
// or company proxy answering on its behalf. Those all clear on their own within
// seconds, so we ask again before doing something irreversible.
const unlinkConfirmations = 3

// session is one connection, from dial to disconnect.
func (c *Client) session(ctx context.Context, socket string) error {
	dialCtx, cancelDial := context.WithTimeout(ctx, 20*time.Second)
	conn, resp, err := websocket.Dial(dialCtx, socket, &websocket.DialOptions{
		HTTPHeader: http.Header{"Authorization": []string{"Bearer " + c.DeviceToken}},
	})
	cancelDial()
	if err != nil {
		if resp != nil && (resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusUnauthorized) {
			return fmt.Errorf("%w (the relay said: %s)", errRejected, resp.Status)
		}
		return err
	}
	defer conn.CloseNow()

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	c.Send(protocol.TypeHello, protocol.Hello{
		ProtocolVersion:   protocol.Version,
		AgentVersion:      c.AgentVersion,
		OS:                c.OS,
		Hostname:          c.Hostname,
		Simulator:         c.Simulator,
		SleepAfterMinutes: c.SleepAfterMinutes,
	})

	errs := make(chan error, 2)
	go func() { errs <- c.writeLoop(ctx, conn) }()
	go func() { errs <- c.readLoop(ctx, conn) }()

	err = <-errs
	cancel()
	return err
}

func (c *Client) writeLoop(ctx context.Context, conn *websocket.Conn) error {
	// Heartbeats prove the connection is alive even when nothing is happening,
	// which on wipe day is most of the time.
	beat := time.NewTicker(10 * time.Second)
	defer beat.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-beat.C:
			raw, err := protocol.Encode(protocol.TypeHeartbeat, protocol.Heartbeat{})
			if err != nil {
				return err
			}
			if err := writeOne(ctx, conn, raw); err != nil {
				return err
			}
		case raw := <-c.out:
			if err := writeOne(ctx, conn, raw); err != nil {
				return err
			}
		}
	}
}

func writeOne(ctx context.Context, conn *websocket.Conn, raw []byte) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	return conn.Write(ctx, websocket.MessageText, raw)
}

func (c *Client) readLoop(ctx context.Context, conn *websocket.Conn) error {
	for {
		_, data, err := conn.Read(ctx)
		if err != nil {
			return err
		}
		var env protocol.Envelope
		if err := json.Unmarshal(data, &env); err != nil {
			c.Log.Warn("relay sent something we couldn't read", "err", err)
			continue
		}
		switch env.Type {
		case protocol.TypeWelcome:
			var w protocol.Welcome
			if err := protocol.Decode(env.Payload, &w); err == nil {
				c.Log.Info("connected to the relay", "device", w.DeviceID)
				c.Handler.OnConnected(w)
			}
		case protocol.TypeAssign:
			var a protocol.Assign
			if err := protocol.Decode(env.Payload, &a); err == nil {
				c.Handler.OnAssign(a.Job)
			}
		case protocol.TypeCancel:
			var cc protocol.Cancel
			if err := protocol.Decode(env.Payload, &cc); err == nil {
				c.Handler.OnCancel(cc)
			}
		case protocol.TypeServerStatus:
			var st protocol.ServerStatus
			if err := protocol.Decode(env.Payload, &st); err == nil {
				c.Handler.OnServerStatus(st)
			}
		case protocol.TypeError:
			var e protocol.Error
			if err := protocol.Decode(env.Payload, &e); err == nil {
				c.Log.Error("the relay rejected us", "message", e.Message)
				return fmt.Errorf("relay: %s", e.Message)
			}
		}
	}
}
