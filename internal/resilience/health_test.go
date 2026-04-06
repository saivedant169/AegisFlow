package resilience

import (
	"testing"
	"time"
)

type stubChecker struct {
	health ComponentHealth
}

func (s *stubChecker) Check() ComponentHealth {
	return s.health
}

func TestHealthRegistry_Register_And_CheckAll(t *testing.T) {
	reg := NewHealthRegistry()
	reg.Register("db", &stubChecker{ComponentHealth{
		Name:      "db",
		Status:    "healthy",
		LastCheck: time.Now(),
		SafeMode:  "read-only",
	}})
	reg.Register("cache", &stubChecker{ComponentHealth{
		Name:      "cache",
		Status:    "healthy",
		LastCheck: time.Now(),
		SafeMode:  "bypass",
	}})

	results := reg.CheckAll()
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
}

func TestHealthRegistry_IsHealthy(t *testing.T) {
	reg := NewHealthRegistry()
	reg.Register("a", &stubChecker{ComponentHealth{Name: "a", Status: "healthy"}})
	reg.Register("b", &stubChecker{ComponentHealth{Name: "b", Status: "healthy"}})
	if !reg.IsHealthy() {
		t.Fatal("expected healthy")
	}

	reg.Register("c", &stubChecker{ComponentHealth{Name: "c", Status: "degraded"}})
	if reg.IsHealthy() {
		t.Fatal("expected not healthy when one component is degraded")
	}
}

func TestHealthRegistry_IsDegraded(t *testing.T) {
	reg := NewHealthRegistry()
	reg.Register("a", &stubChecker{ComponentHealth{Name: "a", Status: "healthy"}})
	if reg.IsDegraded() {
		t.Fatal("all healthy should not be degraded")
	}

	reg.Register("b", &stubChecker{ComponentHealth{Name: "b", Status: "degraded"}})
	if !reg.IsDegraded() {
		t.Fatal("one degraded component should report degraded")
	}
}

func TestHealthRegistry_AllUnhealthy_Not_Degraded(t *testing.T) {
	reg := NewHealthRegistry()
	reg.Register("a", &stubChecker{ComponentHealth{Name: "a", Status: "unhealthy"}})
	reg.Register("b", &stubChecker{ComponentHealth{Name: "b", Status: "unhealthy"}})
	if reg.IsDegraded() {
		t.Fatal("all unhealthy should not report degraded (it is fully down)")
	}
}

func TestHealthRegistry_Empty(t *testing.T) {
	reg := NewHealthRegistry()
	if !reg.IsHealthy() {
		t.Fatal("empty registry should be healthy")
	}
	if reg.IsDegraded() {
		t.Fatal("empty registry should not be degraded")
	}
}
