package behavioral

import (
	"testing"
	"time"

	"github.com/saivedant169/AegisFlow/internal/envelope"
)

// With a finite window, actions older than the window are pruned as they are
// recorded, so history stays bounded by the window rather than growing forever.
func TestRecordActionPrunesExpiredHistory(t *testing.T) {
	sa := NewSessionAnalyzer("sess", DefaultRules(), 0, 1) // 1-minute window
	now := time.Now().UTC()

	// Three stale actions (10 minutes old) and two fresh ones.
	for i := 0; i < 3; i++ {
		sa.RecordAction(makeEnv("old", "shell.ls", "/tmp", envelope.CapRead, envelope.ProtocolMCP, now.Add(-10*time.Minute)))
	}
	sa.RecordAction(makeEnv("fresh-1", "shell.ls", "/a", envelope.CapRead, envelope.ProtocolMCP, now))
	sa.RecordAction(makeEnv("fresh-2", "shell.ls", "/b", envelope.CapRead, envelope.ProtocolMCP, now))

	hist := sa.History()
	if len(hist) != 2 {
		t.Fatalf("expected stale actions pruned, got %d entries", len(hist))
	}
	for _, e := range hist {
		if e.ID == "old" {
			t.Fatal("a stale action survived pruning")
		}
	}
}

// With an unlimited window, the hard cap still bounds memory: only the most
// recent maxSessionHistory actions are retained.
func TestRecordActionEnforcesHardCap(t *testing.T) {
	sa := NewSessionAnalyzer("sess", DefaultRules(), 0, 0) // unlimited window
	now := time.Now().UTC()

	total := maxSessionHistory + 250
	for i := 0; i < total; i++ {
		sa.RecordAction(makeEnv("e", "shell.ls", "/x", envelope.CapRead, envelope.ProtocolMCP, now.Add(time.Duration(i)*time.Millisecond)))
	}

	if got := len(sa.History()); got != maxSessionHistory {
		t.Fatalf("expected history capped at %d, got %d", maxSessionHistory, got)
	}
}

// The two-pointer fan-out detector must still respect the time window: many
// distinct targets spread so that no single window holds the threshold count
// must not alert.
func TestSuspiciousFanOut_SpreadBeyondWindowNoAlert(t *testing.T) {
	rule := SuspiciousFanOut{MaxTargets: 5, WindowSeconds: 10}
	now := time.Now().UTC()

	// 12 distinct targets, each 30s apart => at most 1 per 10s window.
	var history []envelope.ActionEnvelope
	for i := 0; i < 12; i++ {
		target := "host-" + string(rune('a'+i)) + ".example.com"
		history = append(history, *makeEnv("e", "http.get", target, envelope.CapRead, envelope.ProtocolHTTP, now.Add(time.Duration(i)*30*time.Second)))
	}

	if alert := rule.Detect(history); alert != nil {
		t.Fatalf("expected no alert when distinct targets are spread beyond the window, got %+v", alert)
	}
}

// A burst of distinct targets inside the window still alerts after the
// two-pointer rewrite, including when earlier, out-of-window actions precede it.
func TestSuspiciousFanOut_BurstAfterQuietStillAlerts(t *testing.T) {
	rule := SuspiciousFanOut{MaxTargets: 5, WindowSeconds: 10}
	now := time.Now().UTC()

	var history []envelope.ActionEnvelope
	// Two early actions far in the past (must be slid out of the window).
	history = append(history, *makeEnv("old1", "http.get", "old-a", envelope.CapRead, envelope.ProtocolHTTP, now))
	history = append(history, *makeEnv("old2", "http.get", "old-b", envelope.CapRead, envelope.ProtocolHTTP, now.Add(time.Second)))
	// Burst of 6 distinct targets within a 5s window, 5 minutes later.
	burst := now.Add(5 * time.Minute)
	for i := 0; i < 6; i++ {
		target := "burst-" + string(rune('a'+i))
		history = append(history, *makeEnv("e", "http.get", target, envelope.CapRead, envelope.ProtocolHTTP, burst.Add(time.Duration(i)*time.Second)))
	}

	alert := rule.Detect(history)
	if alert == nil {
		t.Fatal("expected an alert for a 6-distinct-target burst inside the window")
	}
	// The reported actions must be only the in-window burst, not the old ones.
	for _, id := range alert.Actions {
		if id == "old1" || id == "old2" {
			t.Fatalf("alert wrongly included an out-of-window action: %v", alert.Actions)
		}
	}
}
