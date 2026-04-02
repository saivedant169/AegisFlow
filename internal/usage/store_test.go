package usage

import (
	"testing"

	"github.com/saivedant169/AegisFlow/pkg/types"
)

func TestStoreAddsProviderBreakdown(t *testing.T) {
	store := NewStore()
	u := types.Usage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15}

	store.Add("tenant-1", "openai", "gpt-4o", u, 1.25)
	store.Add("tenant-1", "openai", "gpt-4o", u, 1.25)
	store.Add("tenant-1", "anthropic", "gpt-4o", u, 0.75)

	got := store.Get("tenant-1")
	if got == nil {
		t.Fatal("expected tenant usage")
	}
	if got.TotalRequests != 3 {
		t.Fatalf("expected 3 total requests, got %d", got.TotalRequests)
	}
	if got.ByModel["gpt-4o"].Requests != 3 {
		t.Fatalf("expected aggregated model requests to be 3, got %d", got.ByModel["gpt-4o"].Requests)
	}
	if len(got.ByProviderModel) != 2 {
		t.Fatalf("expected 2 provider/model entries, got %d", len(got.ByProviderModel))
	}
	openAI := got.ByProviderModel["openai\x00gpt-4o"]
	if openAI == nil || openAI.Requests != 2 {
		t.Fatalf("expected openai entry to have 2 requests, got %+v", openAI)
	}
	anthropic := got.ByProviderModel["anthropic\x00gpt-4o"]
	if anthropic == nil || anthropic.Requests != 1 {
		t.Fatalf("expected anthropic entry to have 1 request, got %+v", anthropic)
	}
}
