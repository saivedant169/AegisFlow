package resilience

import (
	"testing"
	"time"
)

func TestCircuitBreaker_StartsClose(t *testing.T) {
	cb := NewCircuitBreaker("test", 3, 5*time.Second)
	if cb.State() != CircuitClosed {
		t.Fatalf("expected closed, got %s", cb.State())
	}
	if !cb.Allow() {
		t.Fatal("closed circuit should allow")
	}
}

func TestCircuitBreaker_OpensAfterThreshold(t *testing.T) {
	cb := NewCircuitBreaker("test", 3, 5*time.Second)
	cb.RecordFailure()
	cb.RecordFailure()
	if cb.State() != CircuitClosed {
		t.Fatal("should still be closed after 2 failures")
	}
	cb.RecordFailure()
	if cb.State() != CircuitOpen {
		t.Fatalf("expected open after 3 failures, got %s", cb.State())
	}
	if cb.Allow() {
		t.Fatal("open circuit should not allow")
	}
}

func TestCircuitBreaker_HalfOpenAfterReset(t *testing.T) {
	cb := NewCircuitBreaker("test", 1, 10*time.Millisecond)
	cb.RecordFailure()
	if cb.State() != CircuitOpen {
		t.Fatal("should be open")
	}

	time.Sleep(20 * time.Millisecond)
	if !cb.Allow() {
		t.Fatal("should allow after reset period (transition to half-open)")
	}
	if cb.State() != CircuitHalfOpen {
		t.Fatalf("expected half-open, got %s", cb.State())
	}
}

func TestCircuitBreaker_SuccessResetsToClose(t *testing.T) {
	cb := NewCircuitBreaker("test", 1, 10*time.Millisecond)
	cb.RecordFailure()

	time.Sleep(20 * time.Millisecond)
	cb.Allow() // transition to half-open
	cb.RecordSuccess()

	if cb.State() != CircuitClosed {
		t.Fatalf("expected closed after success, got %s", cb.State())
	}
}

func TestCircuitBreaker_HalfOpenFailureReopens(t *testing.T) {
	cb := NewCircuitBreaker("test", 1, 10*time.Millisecond)
	cb.RecordFailure()

	time.Sleep(20 * time.Millisecond)
	cb.Allow() // transition to half-open
	cb.RecordFailure()

	if cb.State() != CircuitOpen {
		t.Fatalf("expected open after half-open failure, got %s", cb.State())
	}
}

func TestCircuitBreaker_Name(t *testing.T) {
	cb := NewCircuitBreaker("my-service", 3, time.Second)
	if cb.Name() != "my-service" {
		t.Fatalf("expected my-service, got %s", cb.Name())
	}
}
