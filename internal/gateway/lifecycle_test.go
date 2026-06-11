package gateway

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/saivedant169/AegisFlow/internal/config"
	"github.com/saivedant169/AegisFlow/internal/middleware"
)

func TestBuildRequestContext_SessionAliasesTenant(t *testing.T) {
	h := &Handler{}
	start := time.Now()

	for _, surface := range []apiSurface{surfaceOpenAI, surfaceAnthropic} {
		req := httptest.NewRequest(http.MethodPost, "/", nil)
		ctx := context.WithValue(req.Context(), middleware.TenantContextKey, &config.TenantConfig{ID: "tenant-x"})
		req = req.WithContext(ctx)

		rc := h.buildRequestContext(req, surface, start)
		if rc.tenantID != "tenant-x" {
			t.Errorf("surface %d: tenantID = %q, want tenant-x", surface, rc.tenantID)
		}
		if rc.sessionID != rc.tenantID {
			t.Errorf("surface %d: sessionID %q must alias tenantID %q so both wires share one analyzer", surface, rc.sessionID, rc.tenantID)
		}
		if rc.surface != surface {
			t.Errorf("surface not preserved: got %d want %d", rc.surface, surface)
		}
	}
}

func TestBuildRequestContext_NoTenant(t *testing.T) {
	h := &Handler{}
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	rc := h.buildRequestContext(req, surfaceOpenAI, time.Now())
	if rc.tenantID != "" || rc.sessionID != "" {
		t.Errorf("expected empty tenant/session with no context, got %q/%q", rc.tenantID, rc.sessionID)
	}
}

func TestApiSurfaceBehavioralTarget(t *testing.T) {
	if surfaceOpenAI.behavioralTarget() != "chat-completion" {
		t.Errorf("openai target = %q", surfaceOpenAI.behavioralTarget())
	}
	if surfaceAnthropic.behavioralTarget() != "messages" {
		t.Errorf("anthropic target = %q", surfaceAnthropic.behavioralTarget())
	}
}
