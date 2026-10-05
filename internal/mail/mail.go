// Package mail sends the few emails QueueUp needs, through Resend.
//
// Two kinds, kept apart on purpose. Send is for email somebody asked for by
// pressing a button: a password reset, the link for their PC. SendMarketing
// is for email nobody asked for — the reminders after somebody signs up and
// does not pay — and it will not send without an unsubscribe link.
//
// The API key comes from the environment, never from the repo, and is scrubbed
// out of anything on its way to a log.
package mail

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

// Sender talks to Resend. A zero Sender is disabled, which is what lets the
// relay run locally, and in every test, without an email provider.
type Sender struct {
	APIKey string
	From   string
	// BaseURL is Resend's API, overridden in tests.
	BaseURL string
	HTTP    *http.Client
}

// FromEnv builds a sender from the environment:
//
//	QUEUEUP_RESEND_KEY   the API key from resend.com
//	QUEUEUP_MAIL_FROM    the From address, e.g. QueueUp <noreply@queueuprust.com>
//
// With no key set, email is switched off and the relay says so at startup.
func FromEnv() *Sender {
	from := os.Getenv("QUEUEUP_MAIL_FROM")
	if from == "" {
		from = "QueueUp <noreply@queueuprust.com>"
	}
	return &Sender{APIKey: os.Getenv("QUEUEUP_RESEND_KEY"), From: from}
}

// Enabled reports whether email can actually be sent.
func (s *Sender) Enabled() bool { return s != nil && s.APIKey != "" }

// ErrDisabled is returned when there is no key, so callers can tell "email is
// not set up" apart from "the send failed".
var ErrDisabled = errors.New("email is not set up on this relay")

// Send delivers one plain-text email that somebody asked for: a password
// reset, the link for their PC. Nothing to opt out of, because they pressed
// the button.
func (s *Sender) Send(ctx context.Context, to, subject, body string) error {
	return s.send(ctx, to, subject, body, nil)
}

// SendMarketing delivers an email that nobody pressed a button for, and so
// carries a way out of it.
//
// The link goes in the body for the person and in a List-Unsubscribe header
// for their mail app, which is what puts Gmail's and Apple Mail's own
// "Unsubscribe" button at the top of the message. That button is the
// difference between somebody leaving and somebody pressing "Report spam",
// which costs every email QueueUp sends afterwards a little of its
// deliverability.
func (s *Sender) SendMarketing(ctx context.Context, to, subject, body, unsubscribeURL string) error {
	if unsubscribeURL == "" {
		// Refuse rather than send: marketing with no way out is the one kind
		// of email this package must not be able to produce.
		return errors.New("a marketing email needs an unsubscribe link")
	}
	return s.send(ctx, to, subject, body, map[string]string{
		"List-Unsubscribe": "<" + unsubscribeURL + ">",
	})
}

func (s *Sender) send(ctx context.Context, to, subject, body string, headers map[string]string) error {
	if !s.Enabled() {
		return ErrDisabled
	}
	base := s.BaseURL
	if base == "" {
		base = "https://api.resend.com"
	}
	msg := map[string]any{
		"from":    s.From,
		"to":      []string{to},
		"subject": subject,
		"text":    body,
	}
	if len(headers) > 0 {
		msg["headers"] = headers
	}
	payload, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		strings.TrimSuffix(base, "/")+"/emails", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.APIKey)

	client := s.HTTP
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("couldn't reach the email service: %s", s.scrub(err.Error()))
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	var out struct {
		Message string `json:"message"`
		Name    string `json:"name"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if out.Message != "" {
		return fmt.Errorf("the email service refused it: %s", s.scrub(out.Message))
	}
	return fmt.Errorf("the email service returned %s", resp.Status)
}

// scrub keeps the API key out of logs and error messages. Learned from the
// Steam key, which leaked into the relay's logs inside a URL.
func (s *Sender) scrub(text string) string {
	if s.APIKey == "" {
		return text
	}
	return strings.ReplaceAll(text, s.APIKey, "REDACTED")
}
