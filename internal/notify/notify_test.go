package notify

import (
	"io"
	"log/slog"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"queueup/internal/store"
)

func testNotifier(t *testing.T, send func(to, subject, body string) error) (*Notifier, []string) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "n.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	var ids []string
	for i := 0; i < 6; i++ {
		acct, _, err := st.CreateAccount(string(rune('a'+i)) + "@example.com")
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, acct.ID)
	}

	return &Notifier{
		Store: st, Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
		SMTPHost: "smtp.example.com", SMTPPort: "587",
		SMTPUser: "u", SMTPPass: "p", SMTPFrom: "queueup@example.com",
		sendMail: send,
	}, ids
}

// Wipe day means a lot of notifications at once. If one slow mail server can
// hold up everybody else's, the phone alerts arrive minutes late, which for
// "you're in the queue, position 12" is the same as not arriving.
func TestOneSlowEmailDoesNotHoldUpEveryoneElse(t *testing.T) {
	const perSend = 300 * time.Millisecond
	var done int64
	var wg sync.WaitGroup

	n, accounts := testNotifier(t, func(to, subject, body string) error {
		time.Sleep(perSend)
		atomic.AddInt64(&done, 1)
		wg.Done()
		return nil
	})

	wg.Add(len(accounts))
	start := time.Now()
	for _, id := range accounts {
		n.Send(id, Message{Title: "In the queue", Body: "position 12"})
	}

	waited := make(chan struct{})
	go func() { wg.Wait(); close(waited) }()
	select {
	case <-waited:
	case <-time.After(10 * time.Second):
		t.Fatalf("only %d of %d notifications went out in ten seconds", atomic.LoadInt64(&done), len(accounts))
	}

	took := time.Since(start)
	if serial := time.Duration(len(accounts)) * perSend; took > serial/2 {
		t.Fatalf("%d notifications took %s; sending them one after another would be %s. "+
			"They are queueing behind each other.", len(accounts), took.Round(time.Millisecond), serial)
	}
}

// A test hook still sees every message, which is what the relay's own tests
// rely on to assert what the player would have been told.
func TestSendHookSeesEveryMessage(t *testing.T) {
	n, accounts := testNotifier(t, func(string, string, string) error { return nil })

	var mu sync.Mutex
	var seen []string
	n.SendHook = func(accountID string, m Message) {
		mu.Lock()
		seen = append(seen, m.Title)
		mu.Unlock()
	}

	for _, id := range accounts {
		n.Send(id, Message{Title: "hello"})
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		got := len(seen)
		mu.Unlock()
		if got == len(accounts) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	mu.Lock()
	defer mu.Unlock()
	t.Fatalf("hook saw %d of %d messages", len(seen), len(accounts))
}
