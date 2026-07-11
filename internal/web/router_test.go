package web

import (
	"testing"
	"time"
)

func TestLoginLimiterLocksAfterFailures(t *testing.T) {
	limiter := newLoginLimiter()
	now := time.Date(2026, 7, 11, 22, 0, 0, 0, time.UTC)
	limiter.now = func() time.Time { return now }

	key := "127.0.0.1"
	for index := 0; index < maxLoginFailures; index++ {
		if !limiter.allow(key) {
			t.Fatalf("attempt %d should be allowed before lockout", index+1)
		}
		limiter.recordFailure(key)
	}

	if limiter.allow(key) {
		t.Fatalf("expected key to be locked after failures")
	}

	now = now.Add(loginLockout)
	if !limiter.allow(key) {
		t.Fatalf("expected key to be allowed after lockout window")
	}
}

func TestLoginLimiterResetClearsFailures(t *testing.T) {
	limiter := newLoginLimiter()
	key := "127.0.0.1"

	for index := 0; index < maxLoginFailures-1; index++ {
		limiter.recordFailure(key)
	}
	limiter.reset(key)

	for index := 0; index < maxLoginFailures; index++ {
		if !limiter.allow(key) {
			t.Fatalf("attempt %d should be allowed after reset", index+1)
		}
		limiter.recordFailure(key)
	}
}
