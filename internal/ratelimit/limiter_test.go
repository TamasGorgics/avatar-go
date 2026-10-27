package ratelimit

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type fakeClock struct {
	now time.Time
}

func (c *fakeClock) Now() time.Time { return c.now }

func (c *fakeClock) Advance(d time.Duration) { c.now = c.now.Add(d) }

func newTestClock() *fakeClock {
	return &fakeClock{now: time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)}
}

func TestNew(t *testing.T) {
	l := New()
	if !l.Allow("192.0.2.1", 1, 1) {
		t.Fatal("first request denied")
	}
}

func TestAllowPerIPHourly(t *testing.T) {
	clock := newTestClock()
	l := NewWithClock(clock.Now)

	if !l.Allow("192.0.2.1", 2, 100) {
		t.Fatal("first request denied")
	}
	if !l.Allow("192.0.2.1", 2, 100) {
		t.Fatal("second request denied")
	}
	if l.Allow("192.0.2.1", 2, 100) {
		t.Fatal("third request allowed, want denied")
	}
	if !l.Allow("192.0.2.2", 2, 100) {
		t.Fatal("other IP denied")
	}

	clock.Advance(61 * time.Minute)
	if !l.Allow("192.0.2.1", 2, 100) {
		t.Fatal("request denied after window expired")
	}
}

func TestAllowGlobalDailyCap(t *testing.T) {
	clock := newTestClock()
	l := NewWithClock(clock.Now)

	if !l.Allow("192.0.2.1", 100, 2) {
		t.Fatal("first request denied")
	}
	if !l.Allow("192.0.2.2", 100, 2) {
		t.Fatal("second request denied")
	}
	if l.Allow("192.0.2.3", 100, 2) {
		t.Fatal("third request allowed, want denied")
	}

	clock.Advance(24 * time.Hour)
	if !l.Allow("192.0.2.3", 100, 2) {
		t.Fatal("request denied after daily reset")
	}
}

func TestAllowPrunesExactlyOneHourOldEntries(t *testing.T) {
	clock := newTestClock()
	l := NewWithClock(clock.Now)

	if !l.Allow("192.0.2.1", 1, 10) {
		t.Fatal("first request denied")
	}
	clock.Advance(time.Hour)
	if !l.Allow("192.0.2.1", 1, 10) {
		t.Fatal("one-hour-old entry was not pruned")
	}
}

func TestAllowConcurrentDailyCap(t *testing.T) {
	clock := newTestClock()
	l := NewWithClock(clock.Now)

	const (
		requests = 100
		daily    = 10
	)
	var (
		wg      sync.WaitGroup
		allowed atomic.Int64
	)
	for i := 0; i < requests; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if l.Allow("192.0.2.1", requests, daily) {
				allowed.Add(1)
			}
		}()
	}
	wg.Wait()

	if got := allowed.Load(); got != daily {
		t.Fatalf("allowed = %d, want %d", got, daily)
	}
}
