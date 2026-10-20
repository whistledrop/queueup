package relay

import (
	"encoding/json"
	"net/http"
	"testing"
)

func publicGet(t *testing.T, r *winbackRig, path string) (int, map[string]any) {
	t.Helper()
	resp, err := r.ts.Client().Get(r.ts.URL + path)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

// The checker is for anybody arriving from a search, so it needs no account.
func TestTheServerSearchIsOpenToAnybody(t *testing.T) {
	r := newWinbackRig(t, "", "")
	code, out := publicGet(t, r, "/api/public/servers?q=rustafied")
	if code != http.StatusOK {
		t.Fatalf("search = %d %v", code, out)
	}
	list, _ := out["servers"].([]any)
	if len(list) != 2 {
		t.Fatalf("found %d servers, want the two Rustafied ones", len(list))
	}
	if _, leaks := list[0].(map[string]any)["favourite"]; leaks {
		t.Fatal("the public answer carries account fields")
	}
}

func TestASearchNeedsTwoLetters(t *testing.T) {
	r := newWinbackRig(t, "", "")
	if code, _ := publicGet(t, r, "/api/public/servers?q=r"); code != http.StatusBadRequest {
		t.Fatalf("one-letter search = %d, want 400", code)
	}
}

// Only servers in the public list are ever contacted. Anything else, including
// an address typed by a stranger, is refused before anything is sent.
func TestAServerNotInTheListIsNeverContacted(t *testing.T) {
	r := newWinbackRig(t, "", "")
	if code, _ := publicGet(t, r, "/api/public/servers/203.0.113.9:28015"); code != http.StatusNotFound {
		t.Fatalf("status for an address not in the list = %d, want 404", code)
	}
}

func TestPublicSearchesAreLimited(t *testing.T) {
	r := newWinbackRig(t, "", "")
	for i := range publicSearchLimit {
		if code, _ := publicGet(t, r, "/api/public/servers?q=eu"); code != http.StatusOK {
			t.Fatalf("search %d = %d, want 200", i+1, code)
		}
	}
	if code, _ := publicGet(t, r, "/api/public/servers?q=eu"); code != http.StatusTooManyRequests {
		t.Fatalf("search over the limit = %d, want 429", code)
	}
}
