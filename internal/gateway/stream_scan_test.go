package gateway

import (
	"strings"
	"testing"

	"github.com/saivedant169/AegisFlow/internal/policy"
)

// recordingSink captures everything the scanner releases and any block.
type recordingSink struct {
	released strings.Builder
	blocked  *policy.Violation
}

func (rs *recordingSink) sink() streamSink {
	return streamSink{
		writeDelta: func(b []byte) { rs.released.Write(b) },
		block:      func(v *policy.Violation) { rs.blocked = v },
	}
}

func feedSplit(sc *streamScanner, parts ...string) bool {
	for _, p := range parts {
		if !sc.Feed([]byte(p)) {
			return false
		}
	}
	return sc.Close()
}

func TestStreamScanner_CleanPassesThrough(t *testing.T) {
	for _, name := range []string{"incremental", "windowed", "nopolicy"} {
		var pe *policy.Engine
		switch name {
		case "incremental":
			pe = policy.NewEngine(nil, []policy.Filter{policy.NewKeywordFilter("k", policy.ActionBlock, []string{"forbidden"})})
		case "windowed":
			pe = policy.NewEngine(nil, []policy.Filter{policy.NewRegexFilter("r", policy.ActionBlock, []string{`forbid+en`})})
		case "nopolicy":
			pe = nil
		}
		rs := &recordingSink{}
		sc := newStreamScanner(pe, rs.sink())
		if !feedSplit(sc, "hello ", "world ", "this is fine") {
			t.Fatalf("%s: clean stream should not block", name)
		}
		if rs.blocked != nil {
			t.Errorf("%s: unexpected block", name)
		}
		if got := rs.released.String(); got != "hello world this is fine" {
			t.Errorf("%s: released %q, want full clean content", name, got)
		}
	}
}

// A keyword split across feeds must block before the violating span is released.
func TestStreamScanner_SplitKeywordNoEgress(t *testing.T) {
	for _, name := range []string{"incremental", "windowed"} {
		var pe *policy.Engine
		if name == "incremental" {
			pe = policy.NewEngine(nil, []policy.Filter{policy.NewKeywordFilter("k", policy.ActionBlock, []string{"forbidden"})})
		} else {
			pe = policy.NewEngine(nil, []policy.Filter{policy.NewRegexFilter("r", policy.ActionBlock, []string{"forbidden"})})
		}
		rs := &recordingSink{}
		sc := newStreamScanner(pe, rs.sink())

		ok := feedSplit(sc, "saying for", "bid", "den now")
		if ok {
			t.Fatalf("%s: expected block on the completed keyword", name)
		}
		if rs.blocked == nil {
			t.Fatalf("%s: expected a recorded violation", name)
		}
		if strings.Contains(rs.released.String(), "forbidden") {
			t.Fatalf("%s: blocked span egressed: %q", name, rs.released.String())
		}
	}
}
