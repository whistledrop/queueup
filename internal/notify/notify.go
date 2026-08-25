// Package notify gets a short message to the user's phone.
//
// Two channels, tried in this order:
//
//   - Web push. Free, instant, works on modern phone browsers once the user
//     taps "turn on notifications". This is the main channel.
//   - Email, as the fallback for the moments push cannot cover. It only works
//     when SMTP settings are configured on the relay; without them it is
//     quietly skipped, and the message still lands whenever the user opens the
//     site, because everything notified is also in the job timeline.
//
// Messages are written for a person, never for a developer. "You're in" beats
// "state transition to in_server".
package notify

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/smtp"
	"strings"
	"sync"
	"time"

	webpush "github.com/SherClockHolmes/webpush-go"

	"queueup/internal/store"
)

// Message is one notification.
type Message struct {
	Title string `json:"title"`
	Body  string `json:"body"`
	// Tag groups messages so a phone shows one "queue position" notification
	// that updates, not a pile of stale ones.
	Tag string `json:"tag"`
	// URL is where tapping the notification goes, relative to the web app.
	URL string `json:"url"`
}

// Notifier fans a message out to everything an account has registered.
type Notifier struct {
	Store *store.Store
	Log   *slog.Logger

	// VAPID identifies this relay to the browsers' push services. Generate once
	// with `relay gen-vapid` and keep in environment variables.
	VAPIDPublicKey  string
	VAPIDPrivateKey string
	// Subject is a contact address the push services may use, "mailto:you@example.com".
	Subject string

	// SMTP settings for the email fallback. All four must be set for email to be
	// attempted at all.
	SMTPHost string
	SMTPPort string
	SMTPUser string
	SMTPPass string
	SMTPFrom string

	// SendHook, when set, receives every message instead of any real channel.
	// Tests use it to assert on what the user would have been told.
	SendHook func(accountID string, m Message)

	// sendPush and sendMail exist so tests can capture what would be sent.
	sendPush func(sub store.PushSubscription, payload []byte) error
	sendMail func(to, subject, body string) error

	mu sync.Mutex
}

// PushConfigured reports whether web push can work.
func (n *Notifier) PushConfigured() bool {
	return n.VAPIDPublicKey != "" && n.VAPIDPrivateKey != ""
}

// EmailConfigured reports whether the email fallback can work.
func (n *Notifier) EmailConfigured() bool {
	return n.SMTPHost != "" && n.SMTPPort != "" && n.SMTPFrom != ""
}

// Send delivers a message to one account, on every channel that is set up.
// It never blocks the caller's request: failures are logged, not returned,
// because a notification that cannot be delivered must not break a join.
func (n *Notifier) Send(accountID string, m Message) {
	go n.deliver(accountID, m)
}

func (n *Notifier) deliver(accountID string, m Message) {
	// The lock covers the shared bookkeeping and the test hook, and is released
	// before anything touches the network.
	//
	// Holding it across a send would put every notification in one queue, and on
	// wipe day that is a lot of notifications: one unreachable mail server would
	// delay everybody else's "you're in the queue, position 12" until it no
	// longer meant anything.
	n.mu.Lock()
	if n.Log == nil {
		n.Log = slog.Default()
	}
	hook := n.SendHook
	log := n.Log
	n.mu.Unlock()

	if hook != nil {
		hook(accountID, m)
		return
	}
	log.Info("notify", "account", accountID, "title", m.Title, "body", m.Body)

	pushed := false
	if n.PushConfigured() {
		pushed = n.pushAll(accountID, m)
	}

	// Email is the fallback, not a duplicate: it only goes when no push landed.
	if !pushed && n.EmailConfigured() {
		n.emailAccount(accountID, m)
	}
}

func (n *Notifier) pushAll(accountID string, m Message) bool {
	subs, err := n.Store.PushSubscriptions(accountID)
	if err != nil {
		n.Log.Error("loading push subscriptions", "err", err)
		return false
	}
	payload, err := json.Marshal(m)
	if err != nil {
		return false
	}
	send := n.sendPush
	if send == nil {
		send = n.webPush
	}
	any := false
	for _, sub := range subs {
		if err := send(sub, payload); err != nil {
			if isGone(err) {
				// The browser unsubscribed or was wiped. Forget it.
				_ = n.Store.RemovePushSubscription(sub.Endpoint)
				n.Log.Info("removed a dead push subscription")
				continue
			}
			n.Log.Warn("push failed", "err", err)
			continue
		}
		any = true
	}
	return any
}

func (n *Notifier) webPush(sub store.PushSubscription, payload []byte) error {
	resp, err := webpush.SendNotificationWithContext(context.Background(), payload,
		&webpush.Subscription{
			Endpoint: sub.Endpoint,
			Keys:     webpush.Keys{P256dh: sub.P256dh, Auth: sub.Auth},
		},
		&webpush.Options{
			Subscriber:      n.Subject,
			VAPIDPublicKey:  n.VAPIDPublicKey,
			VAPIDPrivateKey: n.VAPIDPrivateKey,
			TTL:             120,
		})
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == 404 || resp.StatusCode == 410 {
		return fmt.Errorf("gone: %d", resp.StatusCode)
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("push service said %d", resp.StatusCode)
	}
	return nil
}

func isGone(err error) bool {
	return err != nil && strings.HasPrefix(err.Error(), "gone:")
}

func (n *Notifier) emailAccount(accountID string, m Message) {
	acct, err := n.Store.AccountByID(accountID)
	if err != nil {
		return
	}
	send := n.sendMail
	if send == nil {
		send = n.smtpSend
	}
	body := m.Body + "\n\nQueueUp"
	if err := send(acct.Email, "QueueUp: "+m.Title, body); err != nil {
		n.Log.Warn("email failed", "err", err)
	}
}

// smtpTimeout bounds every stage of sending one email. Without it a mail server
// that accepts the connection and then says nothing holds the sending goroutine
// forever, and they accumulate.
const smtpTimeout = 20 * time.Second

func (n *Notifier) smtpSend(to, subject, body string) error {
	msg := strings.Join([]string{
		"From: " + n.SMTPFrom,
		"To: " + to,
		"Subject: " + subject,
		"",
		body,
	}, "\r\n")
	addr := n.SMTPHost + ":" + n.SMTPPort

	// net/smtp's own SendMail has no timeout of any kind, so the connection is
	// dialled here and given deadlines before the conversation starts.
	conn, err := net.DialTimeout("tcp", addr, smtpTimeout)
	if err != nil {
		return fmt.Errorf("connecting to the mail server: %w", err)
	}
	defer conn.Close()
	if err := conn.SetDeadline(time.Now().Add(smtpTimeout)); err != nil {
		return err
	}

	client, err := smtp.NewClient(conn, n.SMTPHost)
	if err != nil {
		return fmt.Errorf("starting the mail conversation: %w", err)
	}
	defer client.Close()

	if ok, _ := client.Extension("STARTTLS"); ok {
		if err := client.StartTLS(&tls.Config{ServerName: n.SMTPHost}); err != nil {
			return fmt.Errorf("securing the mail connection: %w", err)
		}
	}
	if n.SMTPUser != "" {
		if err := client.Auth(smtp.PlainAuth("", n.SMTPUser, n.SMTPPass, n.SMTPHost)); err != nil {
			return fmt.Errorf("signing in to the mail server: %w", err)
		}
	}
	if err := client.Mail(n.SMTPFrom); err != nil {
		return err
	}
	if err := client.Rcpt(to); err != nil {
		return err
	}
	w, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write([]byte(msg)); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return client.Quit()
}

// GenerateVAPIDKeys makes the one-time key pair for `relay gen-vapid`.
func GenerateVAPIDKeys() (privateKey, publicKey string, err error) {
	return webpush.GenerateVAPIDKeys()
}
