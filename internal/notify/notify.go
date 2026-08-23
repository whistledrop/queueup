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
	"encoding/json"
	"fmt"
	"log/slog"
	"net/smtp"
	"strings"
	"sync"

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
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.Log == nil {
		n.Log = slog.Default()
	}
	if n.SendHook != nil {
		n.SendHook(accountID, m)
		return
	}
	n.Log.Info("notify", "account", accountID, "title", m.Title, "body", m.Body)

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

func (n *Notifier) smtpSend(to, subject, body string) error {
	msg := strings.Join([]string{
		"From: " + n.SMTPFrom,
		"To: " + to,
		"Subject: " + subject,
		"",
		body,
	}, "\r\n")
	addr := n.SMTPHost + ":" + n.SMTPPort
	var auth smtp.Auth
	if n.SMTPUser != "" {
		auth = smtp.PlainAuth("", n.SMTPUser, n.SMTPPass, n.SMTPHost)
	}
	return smtp.SendMail(addr, auth, n.SMTPFrom, []string{to}, []byte(msg))
}

// GenerateVAPIDKeys makes the one-time key pair for `relay gen-vapid`.
func GenerateVAPIDKeys() (privateKey, publicKey string, err error) {
	return webpush.GenerateVAPIDKeys()
}
