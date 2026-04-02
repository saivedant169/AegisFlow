package router

import (
	"context"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/saivedant169/AegisFlow/internal/admin"
	"github.com/saivedant169/AegisFlow/internal/config"
	"github.com/saivedant169/AegisFlow/internal/provider"
	"github.com/saivedant169/AegisFlow/internal/rollout"
	"github.com/saivedant169/AegisFlow/pkg/types"
)

// failingProvider always returns an error on ChatCompletion and ChatCompletionStream.
type failingProvider struct {
	name string
}

func (f *failingProvider) Name() string { return f.name }
func (f *failingProvider) ChatCompletion(_ context.Context, _ *types.ChatCompletionRequest) (*types.ChatCompletionResponse, error) {
	return nil, fmt.Errorf("provider %s always fails", f.name)
}
func (f *failingProvider) ChatCompletionStream(_ context.Context, _ *types.ChatCompletionRequest) (io.ReadCloser, error) {
	return nil, fmt.Errorf("provider %s always fails (stream)", f.name)
}
func (f *failingProvider) Models(_ context.Context) ([]types.Model, error) { return nil, nil }
func (f *failingProvider) EstimateTokens(_ string) int                     { return 0 }
func (f *failingProvider) Healthy(_ context.Context) bool                  { return true }

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

func TestCanaryRouting(t *testing.T) {
	registry := provider.NewRegistry()
	registry.Register(provider.NewMockProvider("openai", 0))
	registry.Register(provider.NewMockProvider("azure", 0))

	routes := []config.RouteConfig{
		{
			Match:     config.RouteMatch{Model: "gpt-*"},
			Providers: []string{"openai", "azure"},
			Strategy:  "priority",
		},
	}

	router := NewRouter(routes, registry)

	store := rollout.NewMemoryStore()
	reqLog := admin.NewRequestLog(100)
	mgr, err := rollout.NewManager(store, reqLog)
	if err != nil {
		t.Fatalf("failed to create rollout manager: %v", err)
	}

	_, err = mgr.CreateRollout(
		"gpt-4o",
		[]string{"openai"},
		"azure",
		[]int{50},
		5*time.Minute,
		0.05,
		500,
	)
	if err != nil {
		t.Fatalf("failed to create rollout: %v", err)
	}

	router.SetRolloutManager(mgr)

	canaryCount := 0
	baselineCount := 0
	total := 100

	for i := 0; i < total; i++ {
		req := &types.ChatCompletionRequest{
			Model:    "gpt-4o",
			Messages: []types.Message{{Role: "user", Content: "Hello"}},
		}

		result, err := router.RouteWithProvider(context.Background(), req)
		if err != nil {
			t.Fatalf("unexpected error on iteration %d: %v", i, err)
		}

		switch result.Provider {
		case "azure":
			canaryCount++
		case "openai":
			baselineCount++
		default:
			t.Fatalf("unexpected provider: %s", result.Provider)
		}
	}

	t.Logf("canary=%d baseline=%d", canaryCount, baselineCount)

	if canaryCount < 30 || canaryCount > 70 {
		t.Errorf("expected canary count between 30-70, got %d", canaryCount)
	}
}

func TestRegionRouting(t *testing.T) {
	registry := provider.NewRegistry()
	registry.Register(provider.NewMockProvider("providerA", 0))
	registry.Register(provider.NewMockProvider("providerB", 0))

	routes := []config.RouteConfig{
		{
			Match:    config.RouteMatch{Model: "gpt-*"},
			Strategy: "priority",
			Regions: []config.RegionConfig{
				{Name: "us", Providers: []string{"providerA"}, Strategy: "priority"},
				{Name: "eu", Providers: []string{"providerB"}, Strategy: "priority"},
			},
		},
	}

	router := NewRouter(routes, registry)

	req := &types.ChatCompletionRequest{
		Model:    "gpt-4o",
		Messages: []types.Message{{Role: "user", Content: "Hello"}},
	}

	result, err := router.RouteWithProvider(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Provider != "providerA" {
		t.Errorf("expected provider 'providerA', got %q", result.Provider)
	}
	if result.Region != "us" {
		t.Errorf("expected region 'us', got %q", result.Region)
	}
}

func TestRegionFailover(t *testing.T) {
	registry := provider.NewRegistry()
	registry.Register(provider.NewMockProvider("providerA", 0))
	registry.Register(provider.NewMockProvider("providerB", 0))

	routes := []config.RouteConfig{
		{
			Match:    config.RouteMatch{Model: "gpt-*"},
			Strategy: "priority",
			Regions: []config.RegionConfig{
				{Name: "us", Providers: []string{"providerA"}, Strategy: "priority"},
				{Name: "eu", Providers: []string{"providerB"}, Strategy: "priority"},
			},
		},
	}

	router := NewRouter(routes, registry)

	// Circuit-break all providers in the "us" region.
	router.circuitBreaker.RecordFailure("providerA")
	router.circuitBreaker.RecordFailure("providerA")
	router.circuitBreaker.RecordFailure("providerA")

	req := &types.ChatCompletionRequest{
		Model:    "gpt-4o",
		Messages: []types.Message{{Role: "user", Content: "Hello"}},
	}

	result, err := router.RouteWithProvider(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Provider != "providerB" {
		t.Errorf("expected failover to 'providerB', got %q", result.Provider)
	}
	if result.Region != "eu" {
		t.Errorf("expected failover to region 'eu', got %q", result.Region)
	}
}

func TestRegionBackwardCompat(t *testing.T) {
	registry := provider.NewRegistry()
	registry.Register(provider.NewMockProvider("openai", 0))
	registry.Register(provider.NewMockProvider("azure", 0))

	routes := []config.RouteConfig{
		{
			Match:     config.RouteMatch{Model: "gpt-*"},
			Providers: []string{"openai", "azure"},
			Strategy:  "priority",
		},
	}

	router := NewRouter(routes, registry)

	req := &types.ChatCompletionRequest{
		Model:    "gpt-4o",
		Messages: []types.Message{{Role: "user", Content: "Hello"}},
	}

	result, err := router.RouteWithProvider(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Provider != "openai" {
		t.Errorf("expected provider 'openai', got %q", result.Provider)
	}
	if result.Region != "" {
		t.Errorf("expected empty region for non-region route, got %q", result.Region)
	}
}

func TestNoCanaryWithoutRollout(t *testing.T) {
	registry := provider.NewRegistry()
	registry.Register(provider.NewMockProvider("openai", 0))
	registry.Register(provider.NewMockProvider("azure", 0))

	routes := []config.RouteConfig{
		{
			Match:     config.RouteMatch{Model: "gpt-*"},
			Providers: []string{"openai", "azure"},
			Strategy:  "priority",
		},
	}

	router := NewRouter(routes, registry)

	// Do NOT set a rollout manager — normal routing should occur.
	req := &types.ChatCompletionRequest{
		Model:    "gpt-4o",
		Messages: []types.Message{{Role: "user", Content: "Hello"}},
	}

	result, err := router.RouteWithProvider(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// With priority strategy and no rollout, first provider (openai) should be used.
	if result.Provider != "openai" {
		t.Errorf("expected provider 'openai', got %q", result.Provider)
	}

	if result.Response == nil {
		t.Fatal("expected non-nil response")
	}
}

// TestRouteWithRegionsAllFail verifies that when every provider across all
// regions fails, the router returns a meaningful error.
func TestRouteWithRegionsAllFail(t *testing.T) {
	registry := provider.NewRegistry()
	registry.Register(&failingProvider{name: "failA"})
	registry.Register(&failingProvider{name: "failB"})

	routes := []config.RouteConfig{
		{
			Match:    config.RouteMatch{Model: "gpt-*"},
			Strategy: "priority",
			Regions: []config.RegionConfig{
				{Name: "us", Providers: []string{"failA"}, Strategy: "priority"},
				{Name: "eu", Providers: []string{"failB"}, Strategy: "priority"},
			},
		},
	}

	router := NewRouter(routes, registry)
	req := &types.ChatCompletionRequest{
		Model:    "gpt-4o",
		Messages: []types.Message{{Role: "user", Content: "Hello"}},
	}

	_, err := router.RouteWithProvider(context.Background(), req)
	if err == nil {
		t.Fatal("expected error when all region providers fail")
	}
}

// TestRouteWithCanaryFailFallsBackToBaseline sets canary to 100% but uses a
// failing canary provider, expecting the router to fall back to baseline.
func TestRouteWithCanaryFailFallsBackToBaseline(t *testing.T) {
	registry := provider.NewRegistry()
	registry.Register(provider.NewMockProvider("openai", 0))
	registry.Register(&failingProvider{name: "bad-canary"})

	routes := []config.RouteConfig{
		{
			Match:     config.RouteMatch{Model: "gpt-*"},
			Providers: []string{"openai"},
			Strategy:  "priority",
		},
	}

	router := NewRouter(routes, registry)

	store := rollout.NewMemoryStore()
	reqLog := admin.NewRequestLog(100)
	mgr, err := rollout.NewManager(store, reqLog)
	if err != nil {
		t.Fatalf("failed to create rollout manager: %v", err)
	}

	_, err = mgr.CreateRollout(
		"gpt-4o",
		[]string{"openai"},
		"bad-canary",
		[]int{100}, // 100% canary — forces canary path every time
		5*time.Minute,
		0.05,
		500,
	)
	if err != nil {
		t.Fatalf("failed to create rollout: %v", err)
	}

	router.SetRolloutManager(mgr)

	req := &types.ChatCompletionRequest{
		Model:    "gpt-4o",
		Messages: []types.Message{{Role: "user", Content: "Hello"}},
	}

	result, err := router.RouteWithProvider(context.Background(), req)
	if err != nil {
		t.Fatalf("expected fallback to baseline, got error: %v", err)
	}
	if result.Provider != "openai" {
		t.Errorf("expected fallback to 'openai', got %q", result.Provider)
	}
}

// TestRouteStreamWithCanary verifies that RouteStream honours canary rollouts.
func TestRouteStreamWithCanary(t *testing.T) {
	registry := provider.NewRegistry()
	registry.Register(provider.NewMockProvider("openai", 0))
	registry.Register(provider.NewMockProvider("azure", 0))

	routes := []config.RouteConfig{
		{
			Match:     config.RouteMatch{Model: "gpt-*"},
			Providers: []string{"openai", "azure"},
			Strategy:  "priority",
		},
	}

	router := NewRouter(routes, registry)

	store := rollout.NewMemoryStore()
	reqLog := admin.NewRequestLog(100)
	mgr, err := rollout.NewManager(store, reqLog)
	if err != nil {
		t.Fatalf("failed to create rollout manager: %v", err)
	}

	_, err = mgr.CreateRollout(
		"gpt-4o",
		[]string{"openai"},
		"azure",
		[]int{100}, // 100% canary
		5*time.Minute,
		0.05,
		500,
	)
	if err != nil {
		t.Fatalf("failed to create rollout: %v", err)
	}

	router.SetRolloutManager(mgr)

	req := &types.ChatCompletionRequest{
		Model:    "gpt-4o",
		Stream:   true,
		Messages: []types.Message{{Role: "user", Content: "Hello"}},
	}

	stream, err := router.RouteStream(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer stream.Close()
}

// TestRouteStreamWithRegions verifies that RouteStream works with region routing.
func TestRouteStreamWithRegions(t *testing.T) {
	registry := provider.NewRegistry()
	registry.Register(provider.NewMockProvider("providerA", 0))
	registry.Register(provider.NewMockProvider("providerB", 0))

	routes := []config.RouteConfig{
		{
			Match:    config.RouteMatch{Model: "gpt-*"},
			Strategy: "priority",
			Regions: []config.RegionConfig{
				{Name: "us", Providers: []string{"providerA"}, Strategy: "priority"},
				{Name: "eu", Providers: []string{"providerB"}, Strategy: "priority"},
			},
		},
	}

	router := NewRouter(routes, registry)
	req := &types.ChatCompletionRequest{
		Model:    "gpt-4o",
		Stream:   true,
		Messages: []types.Message{{Role: "user", Content: "Hello"}},
	}

	stream, err := router.RouteStream(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer stream.Close()
}

// TestRouteStreamRegionsAllFail verifies RouteStream returns error when all
// region providers fail in streaming mode.
func TestRouteStreamRegionsAllFail(t *testing.T) {
	registry := provider.NewRegistry()
	registry.Register(&failingProvider{name: "failA"})
	registry.Register(&failingProvider{name: "failB"})

	routes := []config.RouteConfig{
		{
			Match:    config.RouteMatch{Model: "gpt-*"},
			Strategy: "priority",
			Regions: []config.RegionConfig{
				{Name: "us", Providers: []string{"failA"}, Strategy: "priority"},
				{Name: "eu", Providers: []string{"failB"}, Strategy: "priority"},
			},
		},
	}

	router := NewRouter(routes, registry)
	req := &types.ChatCompletionRequest{
		Model:    "gpt-4o",
		Stream:   true,
		Messages: []types.Message{{Role: "user", Content: "Hello"}},
	}

	_, err := router.RouteStream(context.Background(), req)
	if err == nil {
		t.Fatal("expected error when all region providers fail in stream mode")
	}
}

// TestResolveProvidersNoMatch verifies that resolveProviders returns an error
// when no route matches the given model.
func TestResolveProvidersNoMatch(t *testing.T) {
	registry := provider.NewRegistry()
	registry.Register(provider.NewMockProvider("mock", 0))

	routes := []config.RouteConfig{
		{
			Match:     config.RouteMatch{Model: "gpt-*"},
			Providers: []string{"mock"},
			Strategy:  "priority",
		},
	}

	router := NewRouter(routes, registry)

	req := &types.ChatCompletionRequest{
		Model:    "claude-3", // does not match "gpt-*"
		Messages: []types.Message{{Role: "user", Content: "Hello"}},
	}

	_, err := router.Route(context.Background(), req)
	if err == nil {
		t.Fatal("expected error for unmatched model")
	}
}

// TestMatchRouteMultiplePatterns verifies that matchRoute returns the first
// matching route when multiple patterns could match.
func TestMatchRouteMultiplePatterns(t *testing.T) {
	registry := provider.NewRegistry()
	registry.Register(provider.NewMockProvider("provA", 0))
	registry.Register(provider.NewMockProvider("provB", 0))

	routes := []config.RouteConfig{
		{
			Match:     config.RouteMatch{Model: "gpt-4*"},
			Providers: []string{"provA"},
			Strategy:  "priority",
		},
		{
			Match:     config.RouteMatch{Model: "gpt-*"},
			Providers: []string{"provB"},
			Strategy:  "priority",
		},
	}

	router := NewRouter(routes, registry)

	// "gpt-4o" matches both "gpt-4*" and "gpt-*"; first route should win.
	result, err := router.RouteWithProvider(context.Background(), &types.ChatCompletionRequest{
		Model:    "gpt-4o",
		Messages: []types.Message{{Role: "user", Content: "Hi"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Provider != "provA" {
		t.Errorf("expected first-matching provider 'provA', got %q", result.Provider)
	}

	// "gpt-3.5" matches only "gpt-*".
	result, err = router.RouteWithProvider(context.Background(), &types.ChatCompletionRequest{
		Model:    "gpt-3.5",
		Messages: []types.Message{{Role: "user", Content: "Hi"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Provider != "provB" {
		t.Errorf("expected 'provB' for second pattern, got %q", result.Provider)
	}
}

// TestMatchRouteNoMatch verifies that matchRoute returns nil when no pattern matches.
func TestMatchRouteNoMatch(t *testing.T) {
	router := setupTestRouter()
	// The wildcard "*" route matches everything, so use a router with a specific pattern.
	registry := provider.NewRegistry()
	registry.Register(provider.NewMockProvider("mock", 0))
	restrictedRouter := NewRouter([]config.RouteConfig{
		{
			Match:     config.RouteMatch{Model: "gpt-4*"},
			Providers: []string{"mock"},
			Strategy:  "priority",
		},
	}, registry)

	route := restrictedRouter.matchRoute("claude-3")
	if route != nil {
		t.Error("expected nil route for non-matching model")
	}
	_ = router // suppress unused
}

// TestModelAlias tests NewModelAlias, Resolve, and List.
func TestModelAlias(t *testing.T) {
	aliases := map[string]string{
		"fast":  "gpt-4o-mini",
		"smart": "gpt-4o",
	}
	ma := NewModelAlias(aliases)

	// Resolve known alias.
	if got := ma.Resolve("fast"); got != "gpt-4o-mini" {
		t.Errorf("expected 'gpt-4o-mini', got %q", got)
	}

	// Resolve unknown alias returns original.
	if got := ma.Resolve("unknown"); got != "unknown" {
		t.Errorf("expected 'unknown', got %q", got)
	}

	// List returns all aliases.
	list := ma.List()
	if len(list) != 2 {
		t.Errorf("expected 2 aliases, got %d", len(list))
	}
	if list["smart"] != "gpt-4o" {
		t.Errorf("expected 'gpt-4o' for smart, got %q", list["smart"])
	}
}

// TestModelAliasNilMap verifies NewModelAlias handles nil input.
func TestModelAliasNilMap(t *testing.T) {
	ma := NewModelAlias(nil)
	if got := ma.Resolve("anything"); got != "anything" {
		t.Errorf("expected 'anything', got %q", got)
	}
	if len(ma.List()) != 0 {
		t.Error("expected empty alias list")
	}
}

// TestNewStrategyRoundRobin verifies that NewStrategy("round-robin") returns
// a RoundRobinStrategy.
func TestNewStrategyRoundRobin(t *testing.T) {
	s := NewStrategy("round-robin")
	if _, ok := s.(*RoundRobinStrategy); !ok {
		t.Error("expected RoundRobinStrategy for 'round-robin'")
	}
}

// TestNewStrategyDefault verifies that NewStrategy with an unknown name
// returns PriorityStrategy.
func TestNewStrategyDefault(t *testing.T) {
	s := NewStrategy("unknown")
	if _, ok := s.(*PriorityStrategy); !ok {
		t.Error("expected PriorityStrategy for unknown strategy name")
	}
}

// TestRoundRobinEmptyProviders verifies that RoundRobinStrategy.Select
// handles an empty provider slice gracefully.
func TestRoundRobinEmptyProviders(t *testing.T) {
	s := &RoundRobinStrategy{}
	result := s.Select(nil)
	if len(result) != 0 {
		t.Error("expected empty result for nil providers")
	}
}

// TestRouteStreamCanaryFail verifies that RouteStream falls back when the
// canary provider's stream call fails.
func TestRouteStreamCanaryFail(t *testing.T) {
	registry := provider.NewRegistry()
	registry.Register(provider.NewMockProvider("openai", 0))
	registry.Register(&failingProvider{name: "bad-canary"})

	routes := []config.RouteConfig{
		{
			Match:     config.RouteMatch{Model: "gpt-*"},
			Providers: []string{"openai"},
			Strategy:  "priority",
		},
	}

	router := NewRouter(routes, registry)

	store := rollout.NewMemoryStore()
	reqLog := admin.NewRequestLog(100)
	mgr, _ := rollout.NewManager(store, reqLog)
	mgr.CreateRollout("gpt-4o", []string{"openai"}, "bad-canary", []int{100}, 5*time.Minute, 0.05, 500)
	router.SetRolloutManager(mgr)

	req := &types.ChatCompletionRequest{
		Model:    "gpt-4o",
		Stream:   true,
		Messages: []types.Message{{Role: "user", Content: "Hello"}},
	}

	// The canary always fails, so RouteStream should fall back to flat providers.
	stream, err := router.RouteStream(context.Background(), req)
	if err != nil {
		t.Fatalf("expected fallback to work, got error: %v", err)
	}
	defer stream.Close()
}

// TestTryProvidersAllCircuitBroken verifies that tryProviders returns a
// "no available providers" error when every provider is circuit-broken.
func TestTryProvidersAllCircuitBroken(t *testing.T) {
	registry := provider.NewRegistry()
	registry.Register(provider.NewMockProvider("mock", 0))

	routes := []config.RouteConfig{
		{
			Match:     config.RouteMatch{Model: "*"},
			Providers: []string{"mock"},
			Strategy:  "priority",
		},
	}

	router := NewRouter(routes, registry)

	// Circuit-break the only provider.
	for i := 0; i < 5; i++ {
		router.circuitBreaker.RecordFailure("mock")
	}

	req := &types.ChatCompletionRequest{
		Model:    "gpt-4o",
		Messages: []types.Message{{Role: "user", Content: "Hello"}},
	}

	_, err := router.Route(context.Background(), req)
	if err == nil {
		t.Fatal("expected error when all providers are circuit-broken")
	}
}
