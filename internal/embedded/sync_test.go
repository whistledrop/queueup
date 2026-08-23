package embedded

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// The canonical copies live in configs/ and testdata/, where the docs point.
// The embedded copies must be byte-identical, or the exe on the PC would
// behave differently from every test. If this fails, run:
//
//	./scripts/sync-embedded.sh
func TestEmbeddedCopiesMatchTheCanonicalFiles(t *testing.T) {
	canonical, err := os.ReadFile("../../configs/patterns.json")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(canonical, Patterns()) {
		t.Fatal("internal/embedded/patterns.json is out of date with configs/patterns.json. Run ./scripts/sync-embedded.sh")
	}

	files, err := filepath.Glob("../../testdata/scenarios/*.json")
	if err != nil || len(files) == 0 {
		t.Fatalf("no canonical scenarios found: %v", err)
	}
	if got, want := len(ScenarioNames()), len(files); got != want {
		t.Fatalf("embedded has %d scenarios, testdata has %d. Run ./scripts/sync-embedded.sh", got, want)
	}
	for _, f := range files {
		name := filepath.Base(f)
		canonical, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		emb, err := Scenario(name[:len(name)-len(".json")])
		if err != nil {
			t.Fatalf("scenario %s is not embedded. Run ./scripts/sync-embedded.sh", name)
		}
		if !bytes.Equal(canonical, emb) {
			t.Fatalf("embedded scenario %s is out of date. Run ./scripts/sync-embedded.sh", name)
		}
	}
}
