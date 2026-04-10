package provider

import (
	"testing"
	"time"
)

func TestKeyRotatorRoundRobin(t *testing.T) {
	kr := NewKeyRotator([]string{"key-1", "key-2", "key-3"}, "round-robin", 0)

	seen := map[string]int{}
	for i := 0; i < 9; i++ {
		k, ok := kr.Pick()
		if !ok {
			t.Fatal("expected key, got none")
		}
		seen[k]++
	}

	// All three keys should have been picked an equal number of times.
	for _, key := range []string{"key-1", "key-2", "key-3"} {
		if seen[key] != 3 {
			t.Errorf("expected key %s to be picked 3 times, got %d", key, seen[key])
		}
	}
}

func TestKeyRotatorMarkFailed(t *testing.T) {
	kr := NewKeyRotator([]string{"key-1", "key-2"}, "round-robin", 0)

	kr.MarkFailed("key-1")

	for i := 0; i < 5; i++ {
		k, ok := kr.Pick()
		if !ok {
			t.Fatal("expected key, got none")
		}
		if k == "key-1" {
			t.Error("permanently failed key should never be picked")
		}
	}
}

func TestKeyRotatorMarkRateLimited(t *testing.T) {
	// Use a very short cooldown so the test doesn't need to sleep long.
	cooldown := 50 * time.Millisecond
	kr := NewKeyRotator([]string{"key-1", "key-2"}, "round-robin", cooldown)

	kr.MarkRateLimited("key-1")

	// key-1 should not be picked during cooldown.
	for i := 0; i < 5; i++ {
		k, ok := kr.Pick()
		if !ok {
			t.Fatal("expected key, got none")
		}
		if k == "key-1" {
			t.Error("rate-limited key should not be picked during cooldown")
		}
	}

	// After the cooldown, key-1 should be re-admitted.
	time.Sleep(cooldown + 10*time.Millisecond)

	seen := map[string]bool{}
	for i := 0; i < 10; i++ {
		k, ok := kr.Pick()
		if !ok {
			t.Fatal("expected key after cooldown, got none")
		}
		seen[k] = true
	}
	if !seen["key-1"] {
		t.Error("key-1 should be picked again after cooldown expires")
	}
}

func TestKeyRotatorAllKeysFailed(t *testing.T) {
	kr := NewKeyRotator([]string{"key-1", "key-2"}, "round-robin", 0)

	kr.MarkFailed("key-1")
	kr.MarkFailed("key-2")

	_, ok := kr.Pick()
	if ok {
		t.Error("Pick should return false when all keys are permanently failed")
	}
	if kr.Available() {
		t.Error("Available should return false when all keys are failed")
	}
}

func TestKeyRotatorAllKeysRateLimited(t *testing.T) {
	kr := NewKeyRotator([]string{"key-1", "key-2"}, "round-robin", time.Hour)

	kr.MarkRateLimited("key-1")
	kr.MarkRateLimited("key-2")

	_, ok := kr.Pick()
	if ok {
		t.Error("Pick should return false when all keys are rate-limited")
	}
	if kr.Available() {
		t.Error("Available should return false when all keys are rate-limited")
	}
}

func TestKeyRotatorSingleKey(t *testing.T) {
	kr := NewKeyRotator([]string{"only-key"}, "round-robin", 0)

	for i := 0; i < 3; i++ {
		k, ok := kr.Pick()
		if !ok {
			t.Fatal("expected key, got none")
		}
		if k != "only-key" {
			t.Errorf("expected only-key, got %s", k)
		}
	}
}

func TestKeyRotatorEmptyKeys(t *testing.T) {
	kr := NewKeyRotator([]string{}, "round-robin", 0)

	_, ok := kr.Pick()
	if ok {
		t.Error("Pick should return false for empty rotator")
	}
	if kr.Available() {
		t.Error("Available should return false for empty rotator")
	}
	if kr.Len() != 0 {
		t.Errorf("Len should be 0, got %d", kr.Len())
	}
}

func TestKeyRotatorEmptyStringKeysIgnored(t *testing.T) {
	kr := NewKeyRotator([]string{"", "real-key", ""}, "round-robin", 0)

	if kr.Len() != 1 {
		t.Errorf("empty string keys should be ignored, expected Len 1 got %d", kr.Len())
	}
	k, ok := kr.Pick()
	if !ok || k != "real-key" {
		t.Errorf("expected real-key, got %q ok=%v", k, ok)
	}
}

func TestKeyRotatorLen(t *testing.T) {
	kr := NewKeyRotator([]string{"key-1", "key-2", "key-3"}, "round-robin", 0)
	if kr.Len() != 3 {
		t.Errorf("expected Len 3, got %d", kr.Len())
	}
	kr.MarkFailed("key-1")
	// Len counts all keys regardless of state.
	if kr.Len() != 3 {
		t.Errorf("Len should still be 3 after marking one failed, got %d", kr.Len())
	}
}
