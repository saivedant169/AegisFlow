package usage

import (
	"sync"
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

func TestStoreGetAllRaceSafe(t *testing.T) {
	s := NewStore()
	var wg sync.WaitGroup

	// Writer: keep adding usage for several tenants/models.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 2000; i++ {
			s.Add("t1", "openai", "gpt-4o", types.Usage{PromptTokens: 1, CompletionTokens: 1, TotalTokens: 2}, 0.01)
			s.Add("t2", "anthropic", "claude", types.Usage{PromptTokens: 1, CompletionTokens: 1, TotalTokens: 2}, 0.02)
		}
	}()

	// Readers: iterate the returned snapshot the way the admin/GraphQL handlers
	// do. With live pointers this raced and panicked on the inner maps.
	for r := 0; r < 4; r++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 2000; i++ {
				for _, tu := range s.GetAll() {
					for _, m := range tu.ByModel {
						_ = m.TotalTokens
					}
				}
				if g := s.Get("t1"); g != nil {
					for range g.ByProviderModel {
					}
				}
			}
		}()
	}
	wg.Wait()
}
