package router

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"time"

	"github.com/aegisflow/aegisflow/internal/config"
	"github.com/aegisflow/aegisflow/internal/provider"
	"github.com/aegisflow/aegisflow/pkg/types"
)

type Route struct {
	Pattern    string
	Providers  []string
	Strategy   Strategy
}

type Router struct {
	routes         []Route
	registry       *provider.Registry
	circuitBreaker *CircuitBreaker
}

func NewRouter(cfg []config.RouteConfig, registry *provider.Registry) *Router {
	routes := make([]Route, len(cfg))
	for i, rc := range cfg {
		routes[i] = Route{
			Pattern:   rc.Match.Model,
			Providers: rc.Providers,
			Strategy:  NewStrategy(rc.Strategy),
		}
	}
	return &Router{
		routes:         routes,
		registry:       registry,
		circuitBreaker: NewCircuitBreaker(3, 30*time.Second),
	}
}

func (r *Router) Route(ctx context.Context, req *types.ChatCompletionRequest) (*types.ChatCompletionResponse, error) {
	providers, err := r.resolveProviders(req.Model)
	if err != nil {
		return nil, err
	}

	var lastErr error
	for _, p := range providers {
		if r.circuitBreaker.IsOpen(p.Name()) {
			continue
		}

		resp, err := p.ChatCompletion(ctx, req)
		if err != nil {
			r.circuitBreaker.RecordFailure(p.Name())
			lastErr = err
			continue
		}

		r.circuitBreaker.RecordSuccess(p.Name())
		return resp, nil
	}

	if lastErr != nil {
		return nil, fmt.Errorf("all providers failed, last error: %w", lastErr)
	}
	return nil, fmt.Errorf("no available providers for model %q", req.Model)
}

func (r *Router) RouteStream(ctx context.Context, req *types.ChatCompletionRequest) (io.ReadCloser, error) {
	providers, err := r.resolveProviders(req.Model)
	if err != nil {
		return nil, err
	}

	var lastErr error
	for _, p := range providers {
		if r.circuitBreaker.IsOpen(p.Name()) {
			continue
		}

		stream, err := p.ChatCompletionStream(ctx, req)
		if err != nil {
			r.circuitBreaker.RecordFailure(p.Name())
			lastErr = err
			continue
		}

		r.circuitBreaker.RecordSuccess(p.Name())
		return stream, nil
	}

	if lastErr != nil {
		return nil, fmt.Errorf("all providers failed, last error: %w", lastErr)
	}
	return nil, fmt.Errorf("no available providers for model %q", req.Model)
}

func (r *Router) resolveProviders(model string) ([]provider.Provider, error) {
	for _, route := range r.routes {
		matched, _ := filepath.Match(route.Pattern, model)
		if !matched {
			continue
		}

		var providers []provider.Provider
		for _, name := range route.Providers {
			p, err := r.registry.Get(name)
			if err != nil {
				continue
			}
			providers = append(providers, p)
		}

		if len(providers) == 0 {
			continue
		}

		return route.Strategy.Select(providers), nil
	}

	return nil, fmt.Errorf("no route matched for model %q", model)
}
