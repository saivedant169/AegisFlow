package router

import (
	"context"
	"testing"
	"time"

	"github.com/aegisflow/aegisflow/internal/config"
	"github.com/aegisflow/aegisflow/internal/provider"
	"github.com/aegisflow/aegisflow/pkg/types"
)

func setupTestRouter() *Router {
	registry := provider.NewRegistry()
	registry.Register(provider.NewMockProvider("mock", 0))
	registry.Register(provider.NewMockProvider("backup", 0))

	routes := []config.RouteConfig{
		{
			Match:     config.RouteMatch{Model: "gpt-*"},
			Providers: []string{"mock", "backup"},
			Strategy:  "priority",
		},
		{
			Match:     config.RouteMatch{Model: "*"},
			Providers: []string{"mock"},
			Strategy:  "priority",
		},
	}

	return NewRouter(routes, registry)
}

func TestRouteExactMatch(t *testing.T) {
	r := setupTestRouter()

	req := &types.ChatCompletionRequest{
		Model:    "gpt-4o",
		Messages: []types.Message{{Role: "user", Content: "Hello"}},
	}

	resp, err := r.Route(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}
	if len(resp.Choices) != 1 {
		t.Errorf("expected 1 choice, got %d", len(resp.Choices))
	}
}

func TestRouteWildcard(t *testing.T) {
	r := setupTestRouter()

	req := &types.ChatCompletionRequest{
		Model:    "some-random-model",
		Messages: []types.Message{{Role: "user", Content: "Hello"}},
	}

	resp, err := r.Route(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}
}

func TestRouteStream(t *testing.T) {
	r := setupTestRouter()

	req := &types.ChatCompletionRequest{
		Model:    "gpt-4o",
		Stream:   true,
		Messages: []types.Message{{Role: "user", Content: "Hello"}},
	}

	stream, err := r.RouteStream(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer stream.Close()
}

func TestCircuitBreakerOpens(t *testing.T) {
	cb := NewCircuitBreaker(2, 1*time.Second)

	cb.RecordFailure("test-provider")
	if cb.IsOpen("test-provider") {
		t.Error("circuit should not be open after 1 failure (threshold=2)")
	}

	cb.RecordFailure("test-provider")
	if !cb.IsOpen("test-provider") {
		t.Error("circuit should be open after 2 failures (threshold=2)")
	}
}

func TestCircuitBreakerResets(t *testing.T) {
	cb := NewCircuitBreaker(1, 50*time.Millisecond)

	cb.RecordFailure("test-provider")
	if !cb.IsOpen("test-provider") {
		t.Error("circuit should be open")
	}

	time.Sleep(60 * time.Millisecond)
	if cb.IsOpen("test-provider") {
		t.Error("circuit should have reset after timeout")
	}
}

func TestCircuitBreakerSuccessResets(t *testing.T) {
	cb := NewCircuitBreaker(1, 30*time.Second)

	cb.RecordFailure("test-provider")
	if !cb.IsOpen("test-provider") {
		t.Error("circuit should be open")
	}

	cb.RecordSuccess("test-provider")
	if cb.IsOpen("test-provider") {
		t.Error("circuit should be closed after success")
	}
}

func TestRoundRobinStrategy(t *testing.T) {
	registry := provider.NewRegistry()
	p1 := provider.NewMockProvider("provider-a", 0)
	p2 := provider.NewMockProvider("provider-b", 0)
	registry.Register(p1)
	registry.Register(p2)

	strategy := &RoundRobinStrategy{}
	providers := []provider.Provider{p1, p2}

	first := strategy.Select(providers)
	second := strategy.Select(providers)

	if first[0].Name() == second[0].Name() {
		t.Error("round-robin should rotate the first provider")
	}
}
