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
		PerIPDailyTokens: 100_000,
		GlobalMinute:     20,
		GlobalDaily:      300,
		PerIPConcurrent:  1,
		GlobalConcurrent: 4,
	}
}

func beginModel(l *Limiter, ip string) (bool, string, time.Duration, func()) {
	allowed, reason, retryAfter := l.CheckAttempt(ip)
	if !allowed {
		return false, reason, retryAfter, nil
	}
	allowed, reason, retryAfter, finish := l.ReserveModel(ip, 1)
	if !allowed {
		return false, reason, retryAfter, nil
	}
	return true, "", 0, func() { finish(0, false) }
}

func mustBegin(t *testing.T, l *Limiter, ip string) func() {
	t.Helper()
	allowed, reason, _, release := beginModel(l, ip)
	if !allowed {
		t.Fatalf("model call for %q denied: %s", ip, reason)
	}
	if release == nil {
		t.Fatalf("model call for %q returned nil finish function", ip)
	}
	return release
}

func mustAttempt(t *testing.T, l *Limiter, ip string) {
	t.Helper()
	allowed, reason, retryAfter := l.CheckAttempt(ip)
	if !allowed {
		t.Fatalf("CheckAttempt(%q) denied: %s (retry after %v)", ip, reason, retryAfter)
	}
}

func mustReserve(t *testing.T, l *Limiter, ip string, estimatedTokens int) func(int, bool) {
	t.Helper()
	allowed, reason, retryAfter, finish := l.ReserveModel(ip, estimatedTokens)
	if !allowed {
		t.Fatalf("ReserveModel(%q) denied: %s (retry after %v)", ip, reason, retryAfter)
	}
	if finish == nil {
		t.Fatalf("ReserveModel(%q) returned nil finish function", ip)
	}
	return finish
}

func TestModelAdmissionAllowsAndFinishes(t *testing.T) {
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

	allowed, reason, _, release := beginModel(l, "192.0.2.1")
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

	allowed, reason, _, release := beginModel(l, "192.0.2.1")
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

	allowed, reason, _, release := beginModel(l, "192.0.2.3")
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

	allowed, reason, _, release := beginModel(l, "192.0.2.3")
	if allowed || release != nil || reason != reasonGlobalDaily {
		t.Fatalf("third request = (allowed=%v, reason=%q, release_nil=%v), want denied by %s", allowed, reason, release == nil, reasonGlobalDaily)
	}

	clock.Advance(24 * time.Hour)
	mustBegin(t, l, "192.0.2.3")()
}

func TestAttemptsDoNotConsumeGlobalModelLimits(t *testing.T) {
	clock := newTestClock()
	limits := testLimits()
	limits.GlobalMinute = 1
	limits.GlobalDaily = 1
	l := NewWithClock(clock.Now, limits)

	// Simulate malformed requests: they pass the early attempt limit but never
	// reserve a model call.
	mustAttempt(t, l, "192.0.2.1")
	mustAttempt(t, l, "192.0.2.1")

	finish := mustReserve(t, l, "192.0.2.1", 100)
	finish(100, true)

	mustAttempt(t, l, "192.0.2.2")
	allowed, reason, _, finish := l.ReserveModel("192.0.2.2", 100)
	if allowed || finish != nil || reason != reasonGlobalDaily {
		t.Fatalf("second model admission = (allowed=%v, reason=%q), want denied by %s", allowed, reason, reasonGlobalDaily)
	}
}

func TestGlobalMinuteCountsModelCalls(t *testing.T) {
	clock := newTestClock()
	limits := testLimits()
	limits.GlobalMinute = 1
	limits.GlobalDaily = 100
	l := NewWithClock(clock.Now, limits)

	mustAttempt(t, l, "192.0.2.1")
	mustReserve(t, l, "192.0.2.1", 100)(100, true)

	clock.Advance(20 * time.Second)
	mustAttempt(t, l, "192.0.2.2")
	allowed, reason, retryAfter, finish := l.ReserveModel("192.0.2.2", 100)
	if allowed || finish != nil || reason != reasonGlobalMinute {
		t.Fatalf("second model admission = (allowed=%v, reason=%q), want denied by %s", allowed, reason, reasonGlobalMinute)
	}
	if want := 40 * time.Second; retryAfter != want {
		t.Fatalf("retryAfter = %v, want %v", retryAfter, want)
	}
}

func TestPerIPDailyTokenReservationAndSettlement(t *testing.T) {
	clock := newTestClock()
	limits := testLimits()
	limits.PerIPDailyTokens = 1000
	l := NewWithClock(clock.Now, limits)

	mustAttempt(t, l, "192.0.2.1")
	mustReserve(t, l, "192.0.2.1", 700)(250, true)

	mustAttempt(t, l, "192.0.2.1")
	mustReserve(t, l, "192.0.2.1", 750)(500, true)

	mustAttempt(t, l, "192.0.2.1")
	allowed, reason, retryAfter, finish := l.ReserveModel("192.0.2.1", 251)
	if allowed || finish != nil || reason != reasonPerIPDailyTokens {
		t.Fatalf("over-budget admission = (allowed=%v, reason=%q), want denied by %s", allowed, reason, reasonPerIPDailyTokens)
	}
	if want := 12 * time.Hour; retryAfter != want {
		t.Fatalf("retryAfter = %v, want %v", retryAfter, want)
	}

	l.mu.Lock()
	used := l.perIP["192.0.2.1"].usedTokens
	l.mu.Unlock()
	if used != 750 {
		t.Fatalf("usedTokens = %d, want 750 after settling actual usage", used)
	}
}

func TestPerIPDailyTokenReservationPreventsConcurrentOverbooking(t *testing.T) {
	clock := newTestClock()
	limits := testLimits()
	limits.PerIPDailyTokens = 1000
	limits.PerIPConcurrent = 2
	l := NewWithClock(clock.Now, limits)

	mustAttempt(t, l, "192.0.2.1")
	finish := mustReserve(t, l, "192.0.2.1", 700)
	mustAttempt(t, l, "192.0.2.1")
	allowed, reason, _, deniedFinish := l.ReserveModel("192.0.2.1", 400)
	if allowed || deniedFinish != nil || reason != reasonPerIPDailyTokens {
		t.Fatalf("concurrent over-budget admission = (allowed=%v, reason=%q), want denied by %s", allowed, reason, reasonPerIPDailyTokens)
	}
	finish(0, false) // Missing usage conservatively keeps the reservation.

	mustAttempt(t, l, "192.0.2.1")
	allowed, reason, _, deniedFinish = l.ReserveModel("192.0.2.1", 301)
	if allowed || deniedFinish != nil || reason != reasonPerIPDailyTokens {
		t.Fatalf("missing-usage fallback = (allowed=%v, reason=%q), want denied by %s", allowed, reason, reasonPerIPDailyTokens)
	}
}

func TestPerIPDailyTokenBudgetResetsAtMidnight(t *testing.T) {
	clock := newTestClock()
	limits := testLimits()
	limits.PerIPDailyTokens = 500
	l := NewWithClock(clock.Now, limits)

	mustAttempt(t, l, "192.0.2.1")
	mustReserve(t, l, "192.0.2.1", 500)(0, false)

	clock.Advance(12 * time.Hour)
	mustAttempt(t, l, "192.0.2.1")
	mustReserve(t, l, "192.0.2.1", 500)(500, true)
}

func TestPerIPDailyTokenBudgetGroupsIPv6By64(t *testing.T) {
	clock := newTestClock()
	limits := testLimits()
	limits.PerIPDailyTokens = 1000
	l := NewWithClock(clock.Now, limits)

	mustAttempt(t, l, "2001:db8::1")
	mustReserve(t, l, "2001:db8::1", 700)(700, true)
	mustAttempt(t, l, "2001:db8::2")
	allowed, reason, _, finish := l.ReserveModel("2001:db8::2", 400)
	if allowed || finish != nil || reason != reasonPerIPDailyTokens {
		t.Fatalf("same-/64 admission = (allowed=%v, reason=%q), want denied by %s", allowed, reason, reasonPerIPDailyTokens)
	}
}

func TestRetryAfterPerIPMinute(t *testing.T) {
	clock := newTestClock()
	limits := testLimits()
	limits.PerIPMinute = 2
	limits.PerIPHourly = 100
	l := NewWithClock(clock.Now, limits)

	mustBegin(t, l, "192.0.2.1")()
	clock.Advance(10 * time.Second)
	mustBegin(t, l, "192.0.2.1")()
	clock.Advance(10 * time.Second)

	allowed, reason, retryAfter, release := beginModel(l, "192.0.2.1")
	if allowed || release != nil || reason != reasonPerIPMinute {
		t.Fatalf("third request = (allowed=%v, reason=%q), want denied by %s", allowed, reason, reasonPerIPMinute)
	}
	if want := 40 * time.Second; retryAfter != want {
		t.Fatalf("retryAfter = %v, want %v", retryAfter, want)
	}
}

func TestRetryAfterPerIPHour(t *testing.T) {
	clock := newTestClock()
	limits := testLimits()
	limits.PerIPMinute = 100
	limits.PerIPHourly = 2
	l := NewWithClock(clock.Now, limits)

	mustBegin(t, l, "192.0.2.1")()
	clock.Advance(10 * time.Minute)
	mustBegin(t, l, "192.0.2.1")()
	clock.Advance(10 * time.Minute)

	allowed, reason, retryAfter, release := beginModel(l, "192.0.2.1")
	if allowed || release != nil || reason != reasonPerIPHour {
		t.Fatalf("third request = (allowed=%v, reason=%q), want denied by %s", allowed, reason, reasonPerIPHour)
	}
	if want := 40 * time.Minute; retryAfter != want {
		t.Fatalf("retryAfter = %v, want %v", retryAfter, want)
	}
}

func TestRetryAfterGlobalMinute(t *testing.T) {
	clock := newTestClock()
	limits := testLimits()
	limits.PerIPMinute = 100
	limits.GlobalMinute = 2
	l := NewWithClock(clock.Now, limits)

	mustBegin(t, l, "192.0.2.1")()
	clock.Advance(15 * time.Second)
	mustBegin(t, l, "192.0.2.2")()
	clock.Advance(15 * time.Second)

	allowed, reason, retryAfter, release := beginModel(l, "192.0.2.3")
	if allowed || release != nil || reason != reasonGlobalMinute {
		t.Fatalf("third request = (allowed=%v, reason=%q), want denied by %s", allowed, reason, reasonGlobalMinute)
	}
	if want := 30 * time.Second; retryAfter != want {
		t.Fatalf("retryAfter = %v, want %v", retryAfter, want)
	}
}

func TestRetryAfterGlobalDaily(t *testing.T) {
	clock := newTestClock()
	limits := testLimits()
	limits.PerIPMinute = 100
	limits.GlobalMinute = 100
	limits.GlobalDaily = 2
	l := NewWithClock(clock.Now, limits)

	mustBegin(t, l, "192.0.2.1")()
	mustBegin(t, l, "192.0.2.2")()

	allowed, reason, retryAfter, release := beginModel(l, "192.0.2.3")
	if allowed || release != nil || reason != reasonGlobalDaily {
		t.Fatalf("third request = (allowed=%v, reason=%q), want denied by %s", allowed, reason, reasonGlobalDaily)
	}
	if want := 12 * time.Hour; retryAfter != want {
		t.Fatalf("retryAfter = %v, want %v", retryAfter, want)
	}
}

func TestRetryAfterZeroForConcurrency(t *testing.T) {
	clock := newTestClock()
	limits := testLimits()
	limits.PerIPMinute = 100
	limits.PerIPConcurrent = 1
	l := NewWithClock(clock.Now, limits)

	release := mustBegin(t, l, "192.0.2.1")

	allowed, reason, retryAfter, deniedRelease := beginModel(l, "192.0.2.1")
	if allowed || deniedRelease != nil || reason != reasonPerIPConcurrent {
		t.Fatalf("second request = (allowed=%v, reason=%q), want denied by %s", allowed, reason, reasonPerIPConcurrent)
	}
	if retryAfter != 0 {
		t.Fatalf("retryAfter = %v, want 0 for concurrency", retryAfter)
	}

	release()
}

func TestPerIPConcurrent(t *testing.T) {
	clock := newTestClock()
	limits := testLimits()
	limits.PerIPMinute = 100
	limits.PerIPConcurrent = 1
	l := NewWithClock(clock.Now, limits)

	release := mustBegin(t, l, "192.0.2.1")

	allowed, reason, _, deniedRelease := beginModel(l, "192.0.2.1")
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

	allowed, reason, _, deniedRelease := beginModel(l, "192.0.2.2")
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

	allowed, reason, _, release := beginModel(l, "2001:db8::2")
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
		mustAttempt(t, l, ip)
	}

	clock.Advance(2 * time.Hour)
	mustAttempt(t, l, "192.0.2.4")

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
			limits: Limits{PerIPMinute: 0, PerIPHourly: 1, PerIPDailyTokens: 1, GlobalMinute: 1, GlobalDaily: 1, PerIPConcurrent: 1, GlobalConcurrent: 1},
			reason: reasonPerIPMinute,
		},
		{
			name:   "per ip hourly",
			limits: Limits{PerIPMinute: 1, PerIPHourly: 0, PerIPDailyTokens: 1, GlobalMinute: 1, GlobalDaily: 1, PerIPConcurrent: 1, GlobalConcurrent: 1},
			reason: reasonPerIPHour,
		},
		{
			name:   "global minute",
			limits: Limits{PerIPMinute: 1, PerIPHourly: 1, PerIPDailyTokens: 1, GlobalMinute: 0, GlobalDaily: 1, PerIPConcurrent: 1, GlobalConcurrent: 1},
			reason: reasonGlobalMinute,
		},
		{
			name:   "global daily",
			limits: Limits{PerIPMinute: 1, PerIPHourly: 1, PerIPDailyTokens: 1, GlobalMinute: 1, GlobalDaily: 0, PerIPConcurrent: 1, GlobalConcurrent: 1},
			reason: reasonGlobalDaily,
		},
		{
			name:   "per ip concurrent",
			limits: Limits{PerIPMinute: 1, PerIPHourly: 1, PerIPDailyTokens: 1, GlobalMinute: 1, GlobalDaily: 1, PerIPConcurrent: 0, GlobalConcurrent: 1},
			reason: reasonPerIPConcurrent,
		},
		{
			name:   "global concurrent",
			limits: Limits{PerIPMinute: 1, PerIPHourly: 1, PerIPDailyTokens: 1, GlobalMinute: 1, GlobalDaily: 1, PerIPConcurrent: 1, GlobalConcurrent: 0},
			reason: reasonGlobalConcurrent,
		},
		{
			name:   "per ip daily tokens",
			limits: Limits{PerIPMinute: 1, PerIPHourly: 1, PerIPDailyTokens: 0, GlobalMinute: 1, GlobalDaily: 1, PerIPConcurrent: 1, GlobalConcurrent: 1},
			reason: reasonPerIPDailyTokens,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			l := NewWithClock(newTestClock().Now, tc.limits)
			allowed, reason, _, release := beginModel(l, "192.0.2.1")
			if allowed || release != nil || reason != tc.reason {
				t.Fatalf("model admission = (allowed=%v, reason=%q, finish_nil=%v), want denied by %s", allowed, reason, release == nil, tc.reason)
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
			ok, _, _, release := beginModel(l, "192.0.2.1")
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
			ok, _, _, release := beginModel(l, "192.0.2.1")
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
