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

func testLimits() Limits {
	return Limits{
		PerIPMinute:      3,
		PerIPHourly:      20,
		GlobalMinute:     20,
		GlobalDaily:      300,
		PerIPConcurrent:  1,
		GlobalConcurrent: 4,
	}
}

func mustBegin(t *testing.T, l *Limiter, ip string) func() {
	t.Helper()
	allowed, reason, release := l.Begin(ip)
	if !allowed {
		t.Fatalf("Begin(%q) denied: %s", ip, reason)
	}
	if release == nil {
		t.Fatalf("Begin(%q) returned nil release", ip)
	}
	return release
}

func TestBeginAllowsAndReleases(t *testing.T) {
	clock := newTestClock()
	l := NewWithClock(clock.Now, testLimits())

	release := mustBegin(t, l, "192.0.2.1")
	release()
	mustBegin(t, l, "192.0.2.1")()
}

func TestPerIPMinute(t *testing.T) {
	clock := newTestClock()
	limits := testLimits()
	limits.PerIPMinute = 2
	limits.PerIPHourly = 100
	l := NewWithClock(clock.Now, limits)

	mustBegin(t, l, "192.0.2.1")()
	mustBegin(t, l, "192.0.2.1")()

	allowed, reason, release := l.Begin("192.0.2.1")
	if allowed || release != nil || reason != reasonPerIPMinute {
		t.Fatalf("third request = (allowed=%v, reason=%q, release_nil=%v), want denied by %s", allowed, reason, release == nil, reasonPerIPMinute)
	}

	clock.Advance(61 * time.Second)
	mustBegin(t, l, "192.0.2.1")()
}

func TestPerIPHourly(t *testing.T) {
	clock := newTestClock()
	limits := testLimits()
	limits.PerIPMinute = 100
	limits.PerIPHourly = 2
	l := NewWithClock(clock.Now, limits)

	mustBegin(t, l, "192.0.2.1")()
	mustBegin(t, l, "192.0.2.1")()

	allowed, reason, release := l.Begin("192.0.2.1")
	if allowed || release != nil || reason != reasonPerIPHour {
		t.Fatalf("third request = (allowed=%v, reason=%q, release_nil=%v), want denied by %s", allowed, reason, release == nil, reasonPerIPHour)
	}

	clock.Advance(61 * time.Minute)
	mustBegin(t, l, "192.0.2.1")()
}

func TestGlobalMinute(t *testing.T) {
	clock := newTestClock()
	limits := testLimits()
	limits.PerIPMinute = 100
	limits.GlobalMinute = 2
	l := NewWithClock(clock.Now, limits)

	mustBegin(t, l, "192.0.2.1")()
	mustBegin(t, l, "192.0.2.2")()

	allowed, reason, release := l.Begin("192.0.2.3")
	if allowed || release != nil || reason != reasonGlobalMinute {
		t.Fatalf("third request = (allowed=%v, reason=%q, release_nil=%v), want denied by %s", allowed, reason, release == nil, reasonGlobalMinute)
	}

	clock.Advance(61 * time.Second)
	mustBegin(t, l, "192.0.2.3")()
}

func TestGlobalDaily(t *testing.T) {
	clock := newTestClock()
	limits := testLimits()
	limits.PerIPMinute = 100
	limits.GlobalMinute = 100
	limits.GlobalDaily = 2
	l := NewWithClock(clock.Now, limits)

	mustBegin(t, l, "192.0.2.1")()
	mustBegin(t, l, "192.0.2.2")()

	allowed, reason, release := l.Begin("192.0.2.3")
	if allowed || release != nil || reason != reasonGlobalDaily {
		t.Fatalf("third request = (allowed=%v, reason=%q, release_nil=%v), want denied by %s", allowed, reason, release == nil, reasonGlobalDaily)
	}

	clock.Advance(24 * time.Hour)
	mustBegin(t, l, "192.0.2.3")()
}

func TestPerIPConcurrent(t *testing.T) {
	clock := newTestClock()
	limits := testLimits()
	limits.PerIPMinute = 100
	limits.PerIPConcurrent = 1
	l := NewWithClock(clock.Now, limits)

	release := mustBegin(t, l, "192.0.2.1")

	allowed, reason, deniedRelease := l.Begin("192.0.2.1")
	if allowed || deniedRelease != nil || reason != reasonPerIPConcurrent {
		t.Fatalf("second request = (allowed=%v, reason=%q, release_nil=%v), want denied by %s", allowed, reason, deniedRelease == nil, reasonPerIPConcurrent)
	}

	release()
	mustBegin(t, l, "192.0.2.1")()
}

func TestGlobalConcurrent(t *testing.T) {
	clock := newTestClock()
	limits := testLimits()
	limits.PerIPMinute = 100
	limits.PerIPConcurrent = 100
	limits.GlobalConcurrent = 1
	l := NewWithClock(clock.Now, limits)

	release := mustBegin(t, l, "192.0.2.1")

	allowed, reason, deniedRelease := l.Begin("192.0.2.2")
	if allowed || deniedRelease != nil || reason != reasonGlobalConcurrent {
		t.Fatalf("second request = (allowed=%v, reason=%q, release_nil=%v), want denied by %s", allowed, reason, deniedRelease == nil, reasonGlobalConcurrent)
	}

	release()
	mustBegin(t, l, "192.0.2.2")()
}

func TestReleaseIsIdempotent(t *testing.T) {
	clock := newTestClock()
	l := NewWithClock(clock.Now, testLimits())

	release := mustBegin(t, l, "192.0.2.1")
	release()
	release()

	l.mu.Lock()
	state := l.perIP["192.0.2.1"]
	globalLive := l.globalLive
	l.mu.Unlock()

	if state.active != 0 || globalLive != 0 {
		t.Fatalf("active counters = (%d, %d), want (0, 0)", state.active, globalLive)
	}
	mustBegin(t, l, "192.0.2.1")()
}

func TestIPv6IsLimitedBy64(t *testing.T) {
	clock := newTestClock()
	limits := testLimits()
	limits.PerIPMinute = 1
	limits.PerIPHourly = 100
	l := NewWithClock(clock.Now, limits)

	mustBegin(t, l, "2001:db8::1")()

	allowed, reason, release := l.Begin("2001:db8::2")
	if allowed || release != nil || reason != reasonPerIPMinute {
		t.Fatalf("same /64 = (allowed=%v, reason=%q, release_nil=%v), want denied by %s", allowed, reason, release == nil, reasonPerIPMinute)
	}

	mustBegin(t, l, "2001:db8:0:1::1")()
}

func TestLimitKey(t *testing.T) {
	tests := []struct {
		ip   string
		want string
	}{
		{ip: "192.0.2.1", want: "192.0.2.1"},
		{ip: "::ffff:192.0.2.1", want: "192.0.2.1"},
		{ip: "2001:0db8:0000:0000:0000:0000:0000:0001", want: "2001:db8::/64"},
		{ip: "2001:db8:0:1::1", want: "2001:db8:0:1::/64"},
		{ip: "not-an-ip", want: "not-an-ip"},
	}
	for _, tc := range tests {
		if got := limitKey(tc.ip); got != tc.want {
			t.Errorf("limitKey(%q) = %q, want %q", tc.ip, got, tc.want)
		}
	}
}

func TestSweepRemovesIdleIPs(t *testing.T) {
	clock := newTestClock()
	limits := testLimits()
	limits.PerIPMinute = 100
	l := NewWithClock(clock.Now, limits)

	for _, ip := range []string{"192.0.2.1", "192.0.2.2", "192.0.2.3"} {
		mustBegin(t, l, ip)()
	}

	clock.Advance(2 * time.Hour)
	mustBegin(t, l, "192.0.2.4")()

	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.perIP) != 1 {
		t.Fatalf("perIP entries = %d, want 1 after sweep", len(l.perIP))
	}
}

func TestZeroLimitsDeny(t *testing.T) {
	tests := []struct {
		name   string
		limits Limits
		reason string
	}{
		{
			name:   "per ip minute",
			limits: Limits{PerIPMinute: 0, PerIPHourly: 1, GlobalMinute: 1, GlobalDaily: 1, PerIPConcurrent: 1, GlobalConcurrent: 1},
			reason: reasonPerIPMinute,
		},
		{
			name:   "per ip hourly",
			limits: Limits{PerIPMinute: 1, PerIPHourly: 0, GlobalMinute: 1, GlobalDaily: 1, PerIPConcurrent: 1, GlobalConcurrent: 1},
			reason: reasonPerIPHour,
		},
		{
			name:   "global minute",
			limits: Limits{PerIPMinute: 1, PerIPHourly: 1, GlobalMinute: 0, GlobalDaily: 1, PerIPConcurrent: 1, GlobalConcurrent: 1},
			reason: reasonGlobalMinute,
		},
		{
			name:   "global daily",
			limits: Limits{PerIPMinute: 1, PerIPHourly: 1, GlobalMinute: 1, GlobalDaily: 0, PerIPConcurrent: 1, GlobalConcurrent: 1},
			reason: reasonGlobalDaily,
		},
		{
			name:   "per ip concurrent",
			limits: Limits{PerIPMinute: 1, PerIPHourly: 1, GlobalMinute: 1, GlobalDaily: 1, PerIPConcurrent: 0, GlobalConcurrent: 1},
			reason: reasonPerIPConcurrent,
		},
		{
			name:   "global concurrent",
			limits: Limits{PerIPMinute: 1, PerIPHourly: 1, GlobalMinute: 1, GlobalDaily: 1, PerIPConcurrent: 1, GlobalConcurrent: 0},
			reason: reasonGlobalConcurrent,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			l := NewWithClock(newTestClock().Now, tc.limits)
			allowed, reason, release := l.Begin("192.0.2.1")
			if allowed || release != nil || reason != tc.reason {
				t.Fatalf("Begin() = (allowed=%v, reason=%q, release_nil=%v), want denied by %s", allowed, reason, release == nil, tc.reason)
			}
		})
	}
}

func TestConcurrentDailyCap(t *testing.T) {
	clock := newTestClock()
	limits := testLimits()
	limits.PerIPMinute = 1000
	limits.PerIPHourly = 1000
	limits.GlobalMinute = 1000
	limits.GlobalDaily = 10
	limits.PerIPConcurrent = 1000
	limits.GlobalConcurrent = 1000
	l := NewWithClock(clock.Now, limits)

	const requests = 100
	var (
		wg      sync.WaitGroup
		allowed atomic.Int64
	)
	for i := 0; i < requests; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ok, _, release := l.Begin("192.0.2.1")
			if ok {
				allowed.Add(1)
				release()
			}
		}()
	}
	wg.Wait()

	if got := allowed.Load(); got != int64(limits.GlobalDaily) {
		t.Fatalf("allowed = %d, want %d", got, limits.GlobalDaily)
	}
}

func TestConcurrentSlots(t *testing.T) {
	clock := newTestClock()
	limits := testLimits()
	limits.PerIPMinute = 1000
	limits.PerIPHourly = 1000
	limits.GlobalMinute = 1000
	limits.GlobalDaily = 1000
	limits.PerIPConcurrent = 100
	limits.GlobalConcurrent = 5
	l := NewWithClock(clock.Now, limits)

	const requests = 100
	var (
		wg       sync.WaitGroup
		start    = make(chan struct{})
		acquired atomic.Int64
		releases = make(chan func(), requests)
	)
	for i := 0; i < requests; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			ok, _, release := l.Begin("192.0.2.1")
			if ok {
				acquired.Add(1)
				releases <- release
			}
		}()
	}
	close(start)
	wg.Wait()

	if got := acquired.Load(); got != int64(limits.GlobalConcurrent) {
		t.Fatalf("acquired = %d, want %d", got, limits.GlobalConcurrent)
	}

	for i := 0; i < int(limits.GlobalConcurrent); i++ {
		(<-releases)()
	}
	mustBegin(t, l, "192.0.2.1")()
}
