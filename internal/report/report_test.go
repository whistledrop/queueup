package report

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func fixedNow() time.Time {
	return time.Date(2026, 9, 3, 19, 30, 0, 0, time.UTC)
}

func TestBuildBundlesBothLogs(t *testing.T) {
	dir := t.TempDir()
	agentLog := filepath.Join(dir, "agent.log")
	gameLog := filepath.Join(dir, "output_log.txt")
	os.WriteFile(agentLog, []byte("agent line one\nagent line two\n"), 0o644)
	os.WriteFile(gameLog, []byte("Connecting: 1.2.3.4:28015 (Raknet)\n"), 0o644)

	out := t.TempDir()
	path, err := Build(out, Inputs{
		AgentVersion: "v0.2.1", AgentLogPath: agentLog, GameLogPath: gameLog, Now: fixedNow,
	})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)
	for _, want := range []string{
		"v0.2.1", "agent line two", "Connecting: 1.2.3.4:28015",
		"AGENT LOG", "GAME LOG", "no passwords",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("report is missing %q", want)
		}
	}
	if !strings.Contains(filepath.Base(path), "2026-09-03") {
		t.Errorf("report name %q does not carry the date", filepath.Base(path))
	}
}

// A missing game log is exactly the situation a report is FOR, so it must be
// reported inside the report, not fail the report.
func TestBuildSurvivesAMissingGameLog(t *testing.T) {
	dir := t.TempDir()
	agentLog := filepath.Join(dir, "agent.log")
	os.WriteFile(agentLog, []byte("agent alive\n"), 0o644)

	path, err := Build(t.TempDir(), Inputs{
		AgentVersion: "v0.2.1", AgentLogPath: agentLog,
		GameLogPath: filepath.Join(dir, "nope", "output_log.txt"), Now: fixedNow,
	})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(path)
	if !strings.Contains(string(raw), "could not read it") {
		t.Error("the missing game log is not explained inside the report")
	}
	if !strings.Contains(string(raw), "agent alive") {
		t.Error("the agent log went missing along with the game log")
	}
}

// Big logs are tailed, not shipped whole: the report must stay sendable.
func TestBuildCapsHugeLogs(t *testing.T) {
	dir := t.TempDir()
	big := filepath.Join(dir, "output_log.txt")
	filler := strings.Repeat("older line that should be cut\n", 40000) // ~1.2MB
	os.WriteFile(big, []byte(filler+"the very last line\n"), 0o644)
	agentLog := filepath.Join(dir, "agent.log")
	os.WriteFile(agentLog, []byte("ok\n"), 0o644)

	path, err := Build(t.TempDir(), Inputs{
		AgentVersion: "v0.2.1", AgentLogPath: agentLog, GameLogPath: big, Now: fixedNow,
	})
	if err != nil {
		t.Fatal(err)
	}
	st, _ := os.Stat(path)
	if st.Size() > 2*capBytes {
		t.Fatalf("report is %d bytes; it would be a pain to send", st.Size())
	}
	raw, _ := os.ReadFile(path)
	if !strings.Contains(string(raw), "the very last line") {
		t.Error("the end of the log, the part that matters, was cut")
	}
	if !strings.Contains(string(raw), "this is the last") {
		t.Error("the report does not say it was truncated")
	}
}
