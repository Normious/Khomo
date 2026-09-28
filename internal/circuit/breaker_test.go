package circuit

import (
	"testing"
	"time"
)

func TestOpensAfterThreshold(t *testing.T) {
	b := NewBreaker(1, 3, 1, time.Minute, 1)
	for i := 0; i < 3; i++ {
		if !b.Allow() {
			t.Fatalf("attempt %d should be allowed", i)
		}
		b.RecordFailure()
	}
	if b.Allow() {
		t.Fatal("breaker should be open")
	}
	if b.State() != StateOpen {
		t.Fatalf("expected open, got %s", b.State())
	}
}

func TestHalfOpenRecovery(t *testing.T) {
	b := NewBreaker(1, 2, 2, 50*time.Millisecond, 1)
	b.RecordFailure()
	b.RecordFailure()
	if b.State() != StateOpen {
		t.Fatalf("expected open, got %s", b.State())
	}
	time.Sleep(60 * time.Millisecond)
	if !b.Allow() {
		t.Fatal("should allow half-open trial")
	}
	b.RecordSuccess()
	if !b.Allow() {
		t.Fatal("should allow second half-open trial")
	}
	b.RecordSuccess()
	if b.State() != StateClosed {
		t.Fatalf("expected closed, got %s", b.State())
	}
}

func TestHalfOpenFailureReopens(t *testing.T) {
	b := NewBreaker(1, 1, 1, 20*time.Millisecond, 1)
	b.RecordFailure()
	time.Sleep(30 * time.Millisecond)
	if !b.Allow() {
		t.Fatal("should allow trial")
	}
	b.RecordFailure()
	if b.State() != StateOpen {
		t.Fatalf("expected re-open, got %s", b.State())
	}
}

func TestSuccessResetsFailures(t *testing.T) {
	b := NewBreaker(1, 3, 1, time.Minute, 1)
	b.RecordFailure()
	b.RecordFailure()
	b.RecordSuccess()
	for i := 0; i < 3; i++ {
		b.RecordFailure()
		if i < 2 && !b.Allow() {
			t.Fatalf("attempt %d should still allow", i)
		}
	}
	if b.Allow() {
		t.Fatal("should be open after 3 consecutive failures")
	}
}
