package storage

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestBuildUsageInsert_PlaceholderLayout(t *testing.T) {
	events := []UsageEvent{
		{TenantID: "t1", Model: "m1", Provider: "p1", PromptTokens: 1, CompletionTokens: 2, TotalTokens: 3, EstimatedCostUSD: 0.1, Cached: false, StatusCode: 200, LatencyMs: 10, CreatedAt: time.Unix(0, 0)},
		{TenantID: "t2", Model: "m2", Provider: "p2", PromptTokens: 4, CompletionTokens: 5, TotalTokens: 9, EstimatedCostUSD: 0.2, Cached: true, StatusCode: 200, LatencyMs: 20, CreatedAt: time.Unix(0, 0)},
		{TenantID: "t3", Model: "m3", Provider: "p3", PromptTokens: 6, CompletionTokens: 7, TotalTokens: 13, EstimatedCostUSD: 0.3, Cached: false, StatusCode: 500, LatencyMs: 30, CreatedAt: time.Unix(0, 0)},
	}

	query, args := buildUsageInsert(events)

	// One flattened argument per column per row.
	if want := len(events) * usageInsertColumns; len(args) != want {
		t.Fatalf("expected %d args, got %d", want, len(args))
	}
	// One VALUES group per event (each opens with "($").
	if got := strings.Count(query, "($"); got != len(events) {
		t.Fatalf("expected %d value groups, got %d in %q", len(events), got, query)
	}
	// Placeholders must be contiguous $1..$N with no gaps or repeats.
	for i := 1; i <= len(args); i++ {
		ph := fmt.Sprintf("$%d", i)
		if strings.Count(query, ph+",") == 0 && strings.Count(query, ph+")") == 0 {
			t.Fatalf("missing placeholder %s in %q", ph, query)
		}
	}
	// The highest placeholder must equal the arg count (no over/under-shoot).
	if strings.Contains(query, fmt.Sprintf("$%d", len(args)+1)) {
		t.Fatalf("placeholder index exceeded arg count in %q", query)
	}
	if !strings.HasPrefix(query, "INSERT INTO usage_events") {
		t.Fatalf("unexpected query prefix: %q", query)
	}
}

func TestBuildUsageInsert_ArgOrderMatchesColumns(t *testing.T) {
	e := UsageEvent{TenantID: "tenant", Model: "model", Provider: "prov", PromptTokens: 11, CompletionTokens: 22, TotalTokens: 33, EstimatedCostUSD: 0.44, Cached: true, StatusCode: 201, LatencyMs: 55, CreatedAt: time.Unix(99, 0)}
	_, args := buildUsageInsert([]UsageEvent{e})

	want := []any{"tenant", "model", "prov", 11, 22, 33, 0.44, true, 201, int64(55), time.Unix(99, 0)}
	if len(args) != len(want) {
		t.Fatalf("expected %d args, got %d", len(want), len(args))
	}
	for i := range want {
		if fmt.Sprintf("%v", args[i]) != fmt.Sprintf("%v", want[i]) {
			t.Errorf("arg %d: got %v, want %v", i, args[i], want[i])
		}
	}
}
