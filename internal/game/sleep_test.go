package game

import "testing"

// Real powercfg output, as Windows prints it.
const powercfgNever = `Power Scheme GUID: 381b4222-f694-41f0-9685-ff5bb260df2e  (Balanced)
  Subgroup GUID: 238c9fa8-0aad-41ed-83f4-97be242c8f20  (Sleep)
    Power Setting GUID: 29f6c1db-86da-48c5-9fdb-f2b67b1f44da  (Sleep after)
      Possible Settings units: Seconds
      Current AC Power Setting Index: 0x00000000
      Current DC Power Setting Index: 0x00000384
`

const powercfgThirtyMinutes = `Power Scheme GUID: 381b4222-f694-41f0-9685-ff5bb260df2e  (Balanced)
  Subgroup GUID: 238c9fa8-0aad-41ed-83f4-97be242c8f20  (Sleep)
    Power Setting GUID: 29f6c1db-86da-48c5-9fdb-f2b67b1f44da  (Sleep after)
      Current AC Power Setting Index: 0x00000708
      Current DC Power Setting Index: 0x00000384
`

func TestParseSleepTimeout(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want int
	}{
		{"never sleeps", powercfgNever, SleepNever},
		{"thirty minutes", powercfgThirtyMinutes, 30},               // 0x708 = 1800s
		{"decimal form", "Current AC Power Setting Index: 600", 10}, // some builds print decimal
		// A small non-zero timeout must never round down to "never".
		{"thirty seconds", "Current AC Power Setting Index: 0x0000001E", 1},
		{"ninety seconds", "Current AC Power Setting Index: 0x0000005A", 2},
		{"unreadable", "powercfg is not recognised as a command", SleepUnknown},
		{"empty", "", SleepUnknown},
		{"nonsense value", "Current AC Power Setting Index: banana", SleepUnknown},
	}
	for _, c := range cases {
		if got := parseSleepTimeout(c.in); got != c.want {
			t.Errorf("%s: got %d, want %d", c.name, got, c.want)
		}
	}
}

// The battery timeout must be ignored. A desktop is on mains, and warning about
// a battery setting that will never apply is noise that teaches people to
// ignore warnings.
func TestBatteryTimeoutIsIgnored(t *testing.T) {
	if got := parseSleepTimeout(powercfgNever); got != SleepNever {
		t.Fatalf("got %d; the battery figure (0x384) leaked into the answer", got)
	}
}
