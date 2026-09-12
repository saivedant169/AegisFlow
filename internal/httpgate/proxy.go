package httpgate

import (
	"log"

	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/saivedant169/AegisFlow/internal/approval"
	"github.com/saivedant169/AegisFlow/internal/cleanup"
	"github.com/saivedant169/AegisFlow/internal/envelope"
	"github.com/saivedant169/AegisFlow/internal/evidence"
	"github.com/saivedant169/AegisFlow/internal/toolpolicy"
	"github.com/saivedant169/AegisFlow/pkg/types"
)

// Proxy is an HTTP reverse proxy that intercepts agent API calls,
// evaluates them against tool policies, and records actions in the evidence chain.
type Proxy struct {
	engine *toolpolicy.Engine
	chain  interface {
		SessionID() string
		Record(*envelope.ActionEnvelope) (*evidence.Record, error)
	}
	queue    *approval.Queue
	services []ServiceConfig
	client   *http.Client
}

// NewProxy creates a new HTTP API interceptor proxy.
func NewProxy(engine *toolpolicy.Engine, chain *evidence.SessionChain, queue *approval.Queue, services []ServiceConfig) *Proxy {
	p := &Proxy{
		engine:   engine,
		queue:    queue,
		services: services,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
	if chain != nil {
		p.chain = chain
	}
	return p
}

// ServeHTTP implements http.Handler.
func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if p.chain == nil {
		http.Error(w, "evidence unavailable", http.StatusServiceUnavailable)
		return
	}
	// Match the request to a configured upstream service.
	svc := MatchService(r.Host, r.URL.Path, p.services)
	if svc == nil {
		writeJSON(w, http.StatusNotFound, types.NewErrorResponse(
			http.StatusNotFound, "not_found", "no matching service for path: "+r.URL.Path,
		))
		return
	}

	// Build tool name and infer capability.
	toolName := BuildToolName(svc.Name, r.Method, r.URL.Path)
	capability := methodToCapability(r.Method)

	// Build upstream URL: replace path prefix with upstream.
	upstreamPath := strings.TrimPrefix(r.URL.Path, svc.PathPrefix)
	if upstreamPath == "" {
		upstreamPath = "/"
	}
	targetURL := svc.UpstreamURL + upstreamPath
	if r.URL.RawQuery != "" {
		targetURL += "?" + r.URL.RawQuery
	}

	// Build action envelope.
	actor := envelope.ActorInfo{
		Type:      "agent",
		ID:        "http-proxy",
		SessionID: p.chain.SessionID(),
	}
	env := envelope.NewEnvelope(actor, "http-api-call", envelope.ProtocolHTTP, toolName, targetURL, capability)
	env.Parameters = map[string]any{
		"method":       r.Method,
		"path":         r.URL.Path,
		"host":         r.Host,
		"query_params": r.URL.Query(),
	}

	// Evaluate against tool policy.
	decision := p.engine.Evaluate(env)
	env.PolicyDecision = decision
	// Persist the decision before creating approvals or dispatching upstream.
	if !p.record(w, env) {
		return
	}

	switch decision {
	case envelope.DecisionBlock:
		writeJSON(w, http.StatusForbidden, types.NewErrorResponse(
			http.StatusForbidden, "policy_violation", "request blocked by policy: "+toolName,
		))
		return

	case envelope.DecisionReview:
		if p.queue == nil {
			http.Error(w, "approval queue unavailable", http.StatusServiceUnavailable)
			return
		}
		approvalID, err := p.queue.Submit(env)
		if err != nil {
			writeJSON(w, http.StatusServiceUnavailable, types.NewErrorResponse(
				http.StatusServiceUnavailable, "service_unavailable", "failed to submit for review: "+err.Error(),
			))
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]string{
			"status":      "pending_review",
			"approval_id": approvalID,
			"tool":        toolName,
		})
		return

	case envelope.DecisionAllow:
		// Proxy the request upstream.
		resp, err := p.forwardRequest(r, svc, upstreamPath)
		if err != nil {
			env.Result = &envelope.ActionResult{
				Success: false,
				Error:   err.Error(),
			}
			if !p.record(w, env) {
				return
			}
			writeJSON(w, http.StatusBadGateway, types.NewErrorResponse(
				http.StatusBadGateway, "upstream_error", "upstream request failed: "+err.Error(),
			))
			return
		}
		defer cleanup.Close(resp.Body)

		// Record success in evidence chain.
		env.Result = &envelope.ActionResult{
			Success:    resp.StatusCode >= 200 && resp.StatusCode < 400,
			StatusCode: resp.StatusCode,
		}
		if !p.record(w, env) {
			return
		}

		// Copy upstream response to client.
		for key, values := range resp.Header {
			for _, v := range values {
				w.Header().Add(key, v)
			}
		}
		w.WriteHeader(resp.StatusCode)
		if _, err := io.Copy(w, resp.Body); err != nil {
			log.Print("upstream response copy failed")
			return
		}
		return
	}

	// Fallback: block unknown decisions.
	writeJSON(w, http.StatusForbidden, types.NewErrorResponse(
		http.StatusForbidden, "policy_violation", "unknown policy decision",
	))
}

// record returns false when evidence cannot be persisted. After dispatch, the
// caller must treat this failure as an unknown recorded outcome, not a safe retry.
func (p *Proxy) record(w http.ResponseWriter, env *envelope.ActionEnvelope) bool {
	if _, err := p.chain.Record(env); err != nil {
		http.Error(w, "evidence unavailable; upstream may have executed if dispatch started; do not retry automatically", http.StatusServiceUnavailable)
		return false
	}
	return true
}

// forwardRequest proxies the incoming request to the upstream service.
func (p *Proxy) forwardRequest(r *http.Request, svc *ServiceConfig, upstreamPath string) (*http.Response, error) {
	targetURL := svc.UpstreamURL + upstreamPath
	if r.URL.RawQuery != "" {
		targetURL += "?" + r.URL.RawQuery
	}

	parsed, err := url.Parse(targetURL)
	if err != nil {
		return nil, err
	}

	proxyReq, err := http.NewRequestWithContext(r.Context(), r.Method, parsed.String(), r.Body)
	if err != nil {
		return nil, err
	}

	// Copy headers from original request.
	for key, values := range r.Header {
		for _, v := range values {
			proxyReq.Header.Add(key, v)
		}
	}

	return p.client.Do(proxyReq)
}

// methodToCapability maps HTTP methods to envelope capabilities.
func methodToCapability(method string) envelope.Capability {
	switch strings.ToUpper(method) {
	case "GET", "HEAD", "OPTIONS":
		return envelope.CapRead
	case "POST", "PUT", "PATCH":
		return envelope.CapWrite
	case "DELETE":
		return envelope.CapDelete
	default:
		return envelope.CapExecute
	}
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Print("JSON response write failed")
		return
	}
}
