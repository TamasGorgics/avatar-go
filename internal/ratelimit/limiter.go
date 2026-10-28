package ratelimit

import (
	"net"
	"sync"
	"time"
)

const (
	reasonPerIPMinute      = "per_ip_minute"
	reasonPerIPHour        = "per_ip_hour"
	reasonGlobalMinute     = "global_minute"
	reasonGlobalDaily      = "global_daily"
	reasonPerIPConcurrent  = "per_ip_concurrent"
	reasonGlobalConcurrent = "global_concurrent"

	sweepInterval = 10 * time.Minute
)

// Limits are the configured abuse-protection thresholds. A zero or negative
// value denies every request for that dimension.
type Limits struct {
	PerIPMinute      int
	PerIPHourly      int
	GlobalMinute     int
	GlobalDaily      int
	PerIPConcurrent  int
	GlobalConcurrent int
}

type ipState struct {
	times  []time.Time
	active int
}

type Limiter struct {
	mu          sync.Mutex
	limits      Limits
	now         func() time.Time
	perIP       map[string]*ipState
	globalTimes []time.Time
	day         string
	dayCount    int
	globalLive  int
	lastSweep   time.Time
}

func New(limits Limits) *Limiter {
	return NewWithClock(time.Now, limits)
}

// NewWithClock is useful for tests that need to control the passage of time.
func NewWithClock(now func() time.Time, limits Limits) *Limiter {
	return &Limiter{
		limits: limits,
		now:    now,
		perIP:  map[string]*ipState{},
	}
}

// Begin reserves a rate-limit slot and an active stream slot. On success it
// returns a release function that must be called when the stream finishes.
// On failure release is nil, reason names the limit that was hit, and
// retryAfter is the time until that limit is expected to free up. retryAfter
// is zero when no useful estimate exists, such as for concurrency limits.
func (l *Limiter) Begin(ip string) (allowed bool, reason string, retryAfter time.Duration, release func()) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	l.sweep(now)

	if today := now.Format("2006-01-02"); today != l.day {
		l.day, l.dayCount = today, 0
	}
	if l.limits.GlobalDaily <= 0 {
		return false, reasonGlobalDaily, 0, nil
	}
	if l.dayCount >= l.limits.GlobalDaily {
		return false, reasonGlobalDaily, untilNextDay(now), nil
	}

	cutMinute := now.Add(-time.Minute)
	keptGlobal := l.globalTimes[:0]
	for _, t := range l.globalTimes {
		if t.After(cutMinute) {
			keptGlobal = append(keptGlobal, t)
		}
	}
	l.globalTimes = keptGlobal
	if len(l.globalTimes) >= l.limits.GlobalMinute {
		var oldest time.Time
		if len(l.globalTimes) > 0 {
			oldest = l.globalTimes[0]
		}
		return false, reasonGlobalMinute, until(oldest, time.Minute, now), nil
	}

	key := limitKey(ip)
	state := l.perIP[key]
	if state == nil {
		state = &ipState{}
		l.perIP[key] = state
	}

	cutHour := now.Add(-time.Hour)
	kept := state.times[:0]
	for _, t := range state.times {
		if t.After(cutHour) {
			kept = append(kept, t)
		}
	}
	state.times = kept

	if len(state.times) >= l.limits.PerIPHourly {
		var oldest time.Time
		if len(state.times) > 0 {
			oldest = state.times[0]
		}
		return false, reasonPerIPHour, until(oldest, time.Hour, now), nil
	}

	minuteCount := 0
	var oldestMinute time.Time
	for _, t := range state.times {
		if t.After(cutMinute) {
			minuteCount++
			if oldestMinute.IsZero() {
				oldestMinute = t
			}
		}
	}
	if minuteCount >= l.limits.PerIPMinute {
		return false, reasonPerIPMinute, until(oldestMinute, time.Minute, now), nil
	}
	if state.active >= l.limits.PerIPConcurrent {
		return false, reasonPerIPConcurrent, 0, nil
	}
	if l.globalLive >= l.limits.GlobalConcurrent {
		return false, reasonGlobalConcurrent, 0, nil
	}

	state.times = append(state.times, now)
	state.active++
	l.globalTimes = append(l.globalTimes, now)
	l.globalLive++
	l.dayCount++

	var once sync.Once
	release = func() {
		once.Do(func() {
			l.mu.Lock()
			defer l.mu.Unlock()
			state.active--
			l.globalLive--
		})
	}
	return true, "", 0, release
}

// until returns how long until oldest+window, clamped to zero. Timestamps are
// kept in order, so the oldest entry is the one that frees the next slot.
func until(oldest time.Time, window time.Duration, now time.Time) time.Duration {
	if oldest.IsZero() {
		return 0
	}
	remaining := oldest.Add(window).Sub(now)
	if remaining < 0 {
		return 0
	}
	return remaining
}

func untilNextDay(now time.Time) time.Duration {
	next := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, now.Location())
	if remaining := next.Sub(now); remaining > 0 {
		return remaining
	}
	return 0
}

// sweep removes per-IP state that has no timestamps in the last hour and no
// active streams. Without this, every IP that ever reached the service would
// stay in the map forever.
func (l *Limiter) sweep(now time.Time) {
	if now.Sub(l.lastSweep) < sweepInterval {
		return
	}
	l.lastSweep = now
	cut := now.Add(-time.Hour)
	for key, state := range l.perIP {
		if state.active > 0 {
			continue
		}
		if len(state.times) == 0 || !state.times[len(state.times)-1].After(cut) {
			delete(l.perIP, key)
		}
	}
}

// limitKey groups IPv6 clients by /64 so rotating addresses inside a single
// allocation cannot bypass the per-IP limits. IPv4 keys stay per-address.
func limitKey(ip string) string {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return ip
	}
	if v4 := parsed.To4(); v4 != nil {
		return v4.String()
	}
	return parsed.Mask(net.CIDRMask(64, 128)).String() + "/64"
}
