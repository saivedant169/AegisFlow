package policy

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

// StreamMatcher scans streamed output incrementally and reports the first
// policy violation. Write may be called many times with successive chunks; the
// matcher carries enough state between calls to catch a match that straddles a
// chunk boundary. Once a violation fires it latches: every later Write returns
// that same violation, so a caller that stops on the first non-nil result never
// sees the stream "go clean" again.
//
// Close flushes any bytes still held back for normalization (the trailing
// grapheme of the stream) and runs a final scan. It must be called once at
// end-of-stream; a match that ends on the very last byte is only reported here.
type StreamMatcher interface {
	Write(p []byte) *Violation
	Close() *Violation
}

// StreamableFilter is implemented by filters whose Check can be evaluated
// incrementally over a byte stream with a bounded amount of retained context.
// Filters that can match an unbounded span (arbitrary regex) deliberately do
// not implement it, so the engine falls back to a full-window scan for them.
type StreamableFilter interface {
	Filter
	NewStreamMatcher() StreamMatcher
}

// NewOutputStreamMatcher returns an incremental matcher covering every output
// filter, and true, only when all of them support streaming. If any output
// filter can match an unbounded span (regex/PII/WASM), it returns (nil, false)
// and the caller must keep using the full-window CheckOutput scan.
func (e *Engine) NewOutputStreamMatcher() (StreamMatcher, bool) {
	if len(e.outputFilters) == 0 {
		return nil, false
	}
	matchers := make([]StreamMatcher, 0, len(e.outputFilters))
	for _, f := range e.outputFilters {
		sf, ok := f.(StreamableFilter)
		if !ok {
			return nil, false
		}
		matchers = append(matchers, sf.NewStreamMatcher())
	}
	if len(matchers) == 1 {
		return matchers[0], true
	}
	return &multiStreamMatcher{matchers: matchers}, true
}

// multiStreamMatcher feeds each chunk to every underlying matcher and returns
// the first violation any of them reports.
type multiStreamMatcher struct {
	matchers []StreamMatcher
	latched  *Violation
}

func (m *multiStreamMatcher) Write(p []byte) *Violation {
	if m.latched != nil {
		return m.latched
	}
	for _, sm := range m.matchers {
		if v := sm.Write(p); v != nil {
			m.latched = v
			return v
		}
	}
	return nil
}

func (m *multiStreamMatcher) Close() *Violation {
	if m.latched != nil {
		return m.latched
	}
	for _, sm := range m.matchers {
		if v := sm.Close(); v != nil {
			m.latched = v
			return v
		}
	}
	return nil
}

// keywordStreamMatcher scans output incrementally for normalized keywords.
//
// KeywordFilter matches on text after NFKC + lowercase + whitespace-collapse,
// so a fixed *raw* byte overlap can't bound a straddling match: arbitrary
// whitespace between a keyword's words collapses to a single space, so the raw
// span of a match is unbounded. This matcher instead normalizes incrementally
// and keeps its overlap in *normalized* space — it retains the last `overlap`
// normalized bytes (the longest keyword's length) between writes, which is
// exactly enough for any single keyword that crosses a boundary. Total work is
// O(total bytes) instead of the O(n^2) full-window re-scan.
type keywordStreamMatcher struct {
	name     string
	action   Action
	keywords []string
	overlap  int // normalized-byte context retained between writes

	rawCarry     []byte     // trailing raw bytes that may still combine under NFKC
	tail         []byte     // retained normalized context for straddle detection
	pendingSpace bool       // a whitespace run is pending (collapses to one space)
	started      bool       // emitted a non-space rune yet (for leading-trim)
	latched      *Violation // fires once, then stays
}

// maxRawCarry caps the unnormalized carry so a pathological run with no NFKC
// boundary (e.g. an endless stream of combining marks) can't grow without
// bound. Real grapheme clusters are far shorter than this.
const maxRawCarry = 4096

// NewStreamMatcher returns an incremental matcher for this keyword filter.
func (f *KeywordFilter) NewStreamMatcher() StreamMatcher {
	maxLen := 0
	for _, kw := range f.keywords {
		if len(kw) > maxLen {
			maxLen = len(kw)
		}
	}
	return &keywordStreamMatcher{
		name:     f.name,
		action:   f.action,
		keywords: f.keywords,
		overlap:  maxLen,
	}
}

func (m *keywordStreamMatcher) Write(p []byte) *Violation {
	if m.latched != nil {
		return m.latched
	}

	raw := append(m.rawCarry, p...)

	// Normalize only up to the last NFKC boundary; bytes after it may still
	// combine with what arrives next, so carry them over until the next Write
	// (or Close) can finish them.
	b := norm.NFKC.LastBoundary(raw)
	if b <= 0 {
		if len(raw) <= maxRawCarry {
			m.rawCarry = raw
			return nil
		}
		b = len(raw) // pathological: force-flush so carry stays bounded
	}
	normalized := norm.NFKC.Append(nil, raw[:b]...)
	m.rawCarry = append(m.rawCarry[:0], raw[b:]...)
	return m.scan(normalized)
}

// Close normalizes whatever is still carried (the stream's trailing grapheme)
// and runs the final scan. A keyword that ends on the last byte of the stream
// is only caught here.
func (m *keywordStreamMatcher) Close() *Violation {
	if m.latched != nil {
		return m.latched
	}
	if len(m.rawCarry) == 0 {
		return nil
	}
	normalized := norm.NFKC.Append(nil, m.rawCarry...)
	m.rawCarry = m.rawCarry[:0]
	return m.scan(normalized)
}

// scan folds the normalized bytes (lowercase + whitespace-collapse), appends
// them to the retained context, checks every keyword, and keeps the trailing
// `overlap` bytes for the next straddle check.
func (m *keywordStreamMatcher) scan(normalized []byte) *Violation {
	emit := m.foldChunk(normalized)
	if len(emit) == 0 {
		return nil
	}
	scan := string(m.tail) + string(emit)
	for _, kw := range m.keywords {
		if strings.Contains(scan, kw) {
			m.latched = m.violation(kw)
			return m.latched
		}
	}
	if len(scan) > m.overlap {
		scan = scan[len(scan)-m.overlap:]
	}
	m.tail = append(m.tail[:0], scan...)
	return nil
}

// foldChunk applies the same lowercase + whitespace-collapse that
// normalizeText does, but statefully across chunks: ToLower is per-rune, and a
// pending whitespace run collapses to a single space emitted before the next
// non-space rune (leading and trailing spaces are dropped, matching TrimSpace).
func (m *keywordStreamMatcher) foldChunk(b []byte) []byte {
	out := make([]byte, 0, len(b))
	for i := 0; i < len(b); {
		r, size := utf8.DecodeRune(b[i:])
		i += size
		if isCollapseSpace(r) {
			if m.started {
				m.pendingSpace = true
			}
			continue
		}
		if m.pendingSpace {
			out = append(out, ' ')
			m.pendingSpace = false
		}
		out = utf8.AppendRune(out, unicode.ToLower(r))
		m.started = true
	}
	return out
}

// isCollapseSpace matches RE2's \s class ([\t\n\f\r ]) so the streaming collapse
// is identical to normalizeText's `\s+` regex.
func isCollapseSpace(r rune) bool {
	switch r {
	case '\t', '\n', '\f', '\r', ' ':
		return true
	}
	return false
}

func (m *keywordStreamMatcher) violation(kw string) *Violation {
	if m.action == ActionBlock {
		return &Violation{
			PolicyName: m.name,
			Action:     m.action,
			Message:    fmt.Sprintf("blocked keyword detected: %q", kw),
		}
	}
	return &Violation{
		PolicyName: m.name,
		Action:     m.action,
		Message:    fmt.Sprintf("keyword detected: %q", kw),
	}
}
