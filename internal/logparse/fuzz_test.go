package logparse

import "testing"

// Every line of the Rust log flows through here, and the game writes whatever
// it likes, including player chat and server MOTDs. The parser must survive all
// of it, and a match must never produce an empty user-facing detail.
func FuzzParseLine(f *testing.F) {
	f.Add("You are in queue position 212")
	f.Add("Disconnected: \x00\xff garbage")
	f.Add("Connecting to 1.2.3.4:28015")
	f.Add("queue position 999999999999999999999999")
	f.Add("Shutting down")
	f.Fuzz(func(t *testing.T, line string) {
		p, err := Load("../../configs/patterns.json")
		if err != nil {
			t.Skip("patterns unavailable")
		}
		ev, ok := p.ParseLine(line)
		if ok && ev.Kind != EventUserQuit && ev.Detail == "" {
			t.Fatalf("line %q matched %q but produced no detail for the user", line, ev.PatternID)
		}
		if ev.Position < 0 {
			t.Fatalf("line %q produced negative queue position %d", line, ev.Position)
		}
	})
}
