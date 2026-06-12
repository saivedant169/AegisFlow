package gateway

import (
	"log"
	"strings"

	"github.com/saivedant169/AegisFlow/internal/policy"
)

// streamCheckInterval is the number of buffered output bytes that triggers a
// release; below it, bytes wait so a keyword split across chunks is still caught.
const streamCheckInterval = 500

// streamSink is the wire-specific half of streaming output scanning. writeDelta
// releases a run of bytes that have already passed the policy scan (OpenAI
// passes them through as raw SSE; Anthropic re-frames them as a
// content_block_delta). block renders a terminal policy-violation event.
type streamSink struct {
	writeDelta func(b []byte)
	block      func(v *policy.Violation)
}

// streamScanner enforces the scan-before-release invariant for streamed output:
// bytes are scanned before writeDelta ever sees them, so blocked content cannot
// egress. It is the single shared implementation both the OpenAI and Anthropic
// streaming paths drive, so the security-critical scan can't drift between two
// near-identical copies.
//
// When every output filter supports incremental matching it scans each unit
// once as it arrives (O(total bytes)); otherwise it falls back to re-scanning a
// bounded window with CheckOutput. With no policy configured it just batches and
// releases (nothing to scan).
type streamScanner struct {
	policy      *policy.Engine
	matcher     policy.StreamMatcher
	incremental bool
	pending     []byte
	scanWindow  strings.Builder
	sink        streamSink
}

func newStreamScanner(pe *policy.Engine, sink streamSink) *streamScanner {
	s := &streamScanner{policy: pe, sink: sink}
	if pe != nil {
		if m, ok := pe.NewOutputStreamMatcher(); ok {
			s.matcher = m
			s.incremental = true
		}
	}
	return s
}

// Feed scans one unit of output and buffers it for release, releasing the buffer
// once it reaches the check interval. It returns false if the stream was blocked
// (the sink's block event has already been written and the caller must stop).
func (s *streamScanner) Feed(unit []byte) bool {
	if s.incremental {
		if v := s.matcher.Write(unit); v != nil && v.Action == policy.ActionBlock {
			s.sink.block(v)
			return false
		}
		s.pending = append(s.pending, unit...)
		if len(s.pending) >= streamCheckInterval {
			s.releaseScanned()
		}
		return true
	}

	s.pending = append(s.pending, unit...)
	if s.policy != nil {
		s.scanWindow.Write(unit)
		if s.scanWindow.Len() > maxAccumulatedStreamBytes {
			str := s.scanWindow.String()
			s.scanWindow.Reset()
			s.scanWindow.WriteString(str[len(str)-maxAccumulatedStreamBytes:])
		}
	}
	if len(s.pending) >= streamCheckInterval {
		return s.releaseWindowed()
	}
	return true
}

// Close flushes the matcher's carried trailing bytes (a keyword ending on the
// last byte is only caught here) and releases whatever remains. Returns false if
// the stream was blocked.
func (s *streamScanner) Close() bool {
	if s.incremental {
		if v := s.matcher.Close(); v != nil && v.Action == policy.ActionBlock {
			s.sink.block(v)
			return false
		}
		s.releaseScanned()
		return true
	}
	return s.releaseWindowed()
}

// releaseScanned flushes pending that the incremental matcher already scanned.
func (s *streamScanner) releaseScanned() {
	if len(s.pending) > 0 {
		s.sink.writeDelta(s.pending)
		s.pending = s.pending[:0]
	}
}

// releaseWindowed scans the bounded window before releasing pending. Returns
// false if blocked.
func (s *streamScanner) releaseWindowed() bool {
	if len(s.pending) == 0 {
		return true
	}
	if s.policy != nil {
		if v, checkErr := s.policy.CheckOutput(s.scanWindow.String()); checkErr != nil {
			log.Printf("policy engine stream check error: %v", checkErr)
		} else if v != nil && v.Action == policy.ActionBlock {
			s.sink.block(v)
			return false
		}
	}
	s.sink.writeDelta(s.pending)
	s.pending = s.pending[:0]
	return true
}
