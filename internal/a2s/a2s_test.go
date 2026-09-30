package a2s

import (
	"context"
	"testing"
	"time"
)

func TestQueryReadsPlayersAndQueue(t *testing.T) {
	f, err := NewFakeServer(Info{
		Name: "Rustopia EU Main", Map: "Procedural Map",
		Players: 199, MaxPlayers: 200, Queue: 312,
	}, true)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	got, err := Query(context.Background(), f.Addr(), time.Second)
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if got.Name != "Rustopia EU Main" || got.Players != 199 || got.MaxPlayers != 200 {
		t.Fatalf("info = %+v", got)
	}
	if got.Queue != 312 {
		t.Fatalf("queue = %d, want 312 (keywords were %q)", got.Queue, got.Keywords)
	}
}

// The challenge handshake is what most real servers demand; the query must
// survive it transparently.
func TestQuerySurvivesTheChallengeHandshake(t *testing.T) {
	f, err := NewFakeServer(Info{Name: "x", Players: 1, MaxPlayers: 2}, true)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := Query(context.Background(), f.Addr(), time.Second); err != nil {
		t.Fatalf("Query with challenge: %v", err)
	}
}

// A server that is down for a wipe restart answers nothing. That must come back
// as a quick, plain error, because "down" is the signal the watcher acts on.
func TestQueryTreatsSilenceAsDown(t *testing.T) {
	f, err := NewFakeServer(Info{Name: "x"}, false)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	start := time.Now()
	_, err = Query(context.Background(), f.Addr(), 300*time.Millisecond)
	if err == nil {
		t.Fatal("a silent server produced no error")
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("took %s to give up; the watcher needs a fast answer", elapsed)
	}
}

func TestQueueFromKeywords(t *testing.T) {
	cases := map[string]int{
		"mp200,cp199,qp312":   312,
		"mp200,cp50,qp0":      0,
		"born1755856800,gmn1": 0,  // no qp tag at all
		"qp42":                42, //
		" qp7 ,mp100":         7,  // stray spaces
		"aqp99,mp100":         0,  // aqp is not qp
		"qpnope,mp100":        0,  // not a number
	}
	for keywords, want := range cases {
		if got := QueueFromKeywords(keywords); got != want {
			t.Errorf("QueueFromKeywords(%q) = %d, want %d", keywords, got, want)
		}
	}
}

func TestMalformedRepliesDoNotPanic(t *testing.T) {
	bad := [][]byte{
		nil,
		{0xFF},
		{0xFF, 0xFF, 0xFF, 0xFF, 0x49}, // header only
		{0xFF, 0xFF, 0xFF, 0xFF, 0x49, 17, 'a', 'b'}, // unterminated string
		{0xFF, 0xFF, 0xFF, 0xFF, 0x58, 1, 2, 3},      // wrong type
	}
	for _, b := range bad {
		if _, err := parseInfo(b, "test"); err == nil {
			t.Errorf("parseInfo(%v) accepted garbage", b)
		}
	}
}

// The born tag is what makes a freshly wiped server tell itself apart from one
// that has been up since Tuesday. This is the real tag string from a live Rust
// server, so the parsing is checked against what servers actually send.
func TestBornFromKeywords(t *testing.T) {
	const real = "mp200,cp54,ptrak,qp312,$r?,v2632,^m,^v,EU,^t,born1786038933,gmrust"
	got := BornFromKeywords(real)
	if want := time.Unix(1786038933, 0).UTC(); !got.Equal(want) {
		t.Errorf("born = %v, want %v", got, want)
	}

	for _, keywords := range []string{
		"mp200,cp199,qp312", // no born tag at all
		"bornsoon,mp100",    // not a number
		"born0",             // nonsense
		"born-5",            // nonsense
		"reborn1755856800",  // not our tag
		"",                  // nothing
	} {
		if got := BornFromKeywords(keywords); !got.IsZero() {
			t.Errorf("BornFromKeywords(%q) = %v, want zero", keywords, got)
		}
	}

	// Spacing varies between hosts.
	if got := BornFromKeywords(" born1755856800 ,mp100"); got.Unix() != 1755856800 {
		t.Errorf("a spaced tag did not parse: %v", got)
	}
}
