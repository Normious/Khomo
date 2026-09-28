package ratelimit

import (
	"testing"
)

func TestAllowsUpToBurst(t *testing.T) {
	l := NewLimiter()
	allowed := 0
	for i := 0; i < 10; i++ {
		if l.Allow("k1", 5) {
			allowed++
		}
	}
	if allowed != 5 {
		t.Fatalf("expected 5 allowed, got %d", allowed)
	}
}

func TestUnlimitedWhenZero(t *testing.T) {
	l := NewLimiter()
	for i := 0; i < 100; i++ {
		if !l.Allow("k", 0) {
			t.Fatal("expected unlimited")
		}
	}
}

func TestKeysAreIndependent(t *testing.T) {
	l := NewLimiter()
	for i := 0; i < 3; i++ {
		if !l.Allow("a", 3) {
			t.Fatal("a should allow")
		}
	}
	if l.Allow("a", 3) {
		t.Fatal("a should be exhausted")
	}
	if !l.Allow("b", 3) {
		t.Fatal("b should be independent")
	}
}
