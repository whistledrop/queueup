package game

import "fmt"

// RustAppID is Rust's Steam application id.
const RustAppID = "252490"

// SteamConnectURI builds the string that launches Rust straight into a server.
//
// This is the single most important unverified assumption in the whole product,
// so it lives alone in one function. If the exact format turns out to be wrong,
// this is the only line that changes. See docs/steam-uri-test.md for the manual
// test that confirms it.
func SteamConnectURI(a Addr) string {
	return fmt.Sprintf("steam://run/%s//+connect %s/", RustAppID, a)
}

// URIVariant is one candidate launch string to try by hand.
type URIVariant struct {
	Name string
	URI  string
}

// URIVariants lists the formats worth testing manually, best guess first. The
// agent only ever uses SteamConnectURI; this exists so the manual test in
// docs/steam-uri-test.md has an exact list to work through.
func URIVariants(a Addr) []URIVariant {
	return []URIVariant{
		{"A: space, trailing slash", fmt.Sprintf("steam://run/%s//+connect %s/", RustAppID, a)},
		{"B: space, no trailing slash", fmt.Sprintf("steam://run/%s//+connect %s", RustAppID, a)},
		{"C: url-encoded space", fmt.Sprintf("steam://run/%s//+connect%%20%s/", RustAppID, a)},
		{"D: rungameid form", fmt.Sprintf("steam://rungameid/%s//+connect %s/", RustAppID, a)},
		{"E: connect only (Steam picks the game)", fmt.Sprintf("steam://connect/%s", a)},
	}
}
