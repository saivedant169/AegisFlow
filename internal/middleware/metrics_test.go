package middleware

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestRecordPolicyDecision(t *testing.T) {
	cases := []struct {
		decision string
		protocol string
	}{
		{"allow", "mcp"},
		{"review", "mcp"},
		{"block", "mcp"},
		{"allow", "git"},
	}
	for _, c := range cases {
		before := testutil.ToFloat64(policyDecisionsTotal.WithLabelValues(c.decision, c.protocol))
		RecordPolicyDecision(c.decision, c.protocol)
		after := testutil.ToFloat64(policyDecisionsTotal.WithLabelValues(c.decision, c.protocol))
		if after != before+1 {
			t.Fatalf("decision=%s protocol=%s: counter %v -> %v, want +1", c.decision, c.protocol, before, after)
		}
	}
}

func TestRecordPolicyDecision_EmptyLabelsDefaulted(t *testing.T) {
	before := testutil.ToFloat64(policyDecisionsTotal.WithLabelValues("unknown", "unknown"))
	RecordPolicyDecision("", "")
	after := testutil.ToFloat64(policyDecisionsTotal.WithLabelValues("unknown", "unknown"))
	if after != before+1 {
		t.Fatalf("empty labels: counter %v -> %v, want +1 on unknown/unknown", before, after)
	}
}
