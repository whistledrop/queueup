package a2s

import "testing"

// The relay parses these replies straight off the internet, from servers it does
// not control. A panic in here would take the relay down for everybody, so the
// parser has to survive absolutely anything.
func FuzzParseInfo(f *testing.F) {
	f.Add([]byte{})
	f.Add([]byte{0xFF, 0xFF, 0xFF, 0xFF})
	f.Add([]byte{0xFF, 0xFF, 0xFF, 0xFF, 'I'})
	f.Add([]byte("\xFF\xFF\xFF\xFFI\x11Rust Server\x00Procedural\x00rust\x00Rust\x00"))
	f.Add([]byte("\xFF\xFF\xFF\xFFA\x01\x02\x03\x04"))

	f.Fuzz(func(t *testing.T, data []byte) {
		// The only requirement is that it returns rather than panics. Whether it
		// finds anything useful in random bytes is not the point.
		_, _ = parseInfo(data, "127.0.0.1:28015")
	})
}

// The keyword string is server-authored text and gets parsed for the queue
// count, so it is untrusted in exactly the same way.
func FuzzQueueFromKeywords(f *testing.F) {
	f.Add("mp5,queue3,vanilla")
	f.Add("queue")
	f.Add("queue999999999999999999999999")
	f.Add("")
	f.Fuzz(func(t *testing.T, s string) {
		if n := QueueFromKeywords(s); n < 0 {
			t.Fatalf("negative queue %d from %q", n, s)
		}
	})
}
