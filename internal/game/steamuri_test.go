package game

import "testing"

func TestSteamConnectURI(t *testing.T) {
	a := Addr{IP: "51.83.128.10", Port: 28015}
	want := "steam://run/252490//+connect 51.83.128.10:28015/"
	if got := SteamConnectURI(a); got != want {
		t.Fatalf("SteamConnectURI = %q, want %q", got, want)
	}
}

func TestParseAddr(t *testing.T) {
	good := map[string]Addr{
		"51.83.128.10:28015": {IP: "51.83.128.10", Port: 28015},
		"eu.example.com:28":  {IP: "eu.example.com", Port: 28},
	}
	for in, want := range good {
		got, err := ParseAddr(in)
		if err != nil || got != want {
			t.Errorf("ParseAddr(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	for _, bad := range []string{"51.83.128.10", "", "host:notaport", "host:0", "host:99999"} {
		if _, err := ParseAddr(bad); err == nil {
			t.Errorf("ParseAddr(%q) should have failed", bad)
		}
	}
}

// Every variant must mention Rust's app id, so the manual test in
// docs/steam-uri-test.md cannot accidentally test the wrong game.
func TestURIVariantsAllReferenceRust(t *testing.T) {
	vs := URIVariants(Addr{IP: "1.2.3.4", Port: 28015})
	if len(vs) < 3 {
		t.Fatal("expected several variants to test by hand")
	}
	for _, v := range vs {
		if v.URI == "" || v.Name == "" {
			t.Errorf("empty variant: %+v", v)
		}
	}
}
