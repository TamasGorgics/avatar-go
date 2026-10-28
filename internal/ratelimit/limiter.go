package ratelimit

import (
	"net"
	"sync"
	"time"
)

const (
	reasonPerIPMinute      = "per_ip_minute"
	reasonPerIPHour        = "per_ip_hour"
	reasonPerIPDailyTokens = "per_ip_daily_tokens"
	reasonGlobalMinute     = "global_minute"
	reasonGlobalDaily      = "global_daily"
	reasonPerIPConcurrent  = "per_ip_concurrent"
	reasonGlobalConcurrent = "global_concurrent"

	sweepInterval = 10 * time.Minute
)

// Limits are the configured request-attempt and model-admission thresholds. A
// zero or negative value denies every request for that dimension.
type Limits struct {
	PerIPMinute      int
	PerIPHourly      int
	PerIPDailyTokens int
	GlobalMinute     int
	GlobalDaily      int
	PerIPConcurrent  int
	GlobalConcurrent int
}

type ipState struct {
	attempts   []time.Time
	active     int
	tokenDay   string
	usedTokens int
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

// CheckAttempt applies the per-IP request-attempt limits. Call it before
// parsing the body so malformed requests still consume anti-abuse quota.
func (l *Limiter) CheckAttempt(ip string) (allowed bool, reason string, retryAfter time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	l.sweep(now)
	state := l.stateFor(ip, now)
	l.pruneAttempts(state, now)

	if len(state.attempts) >= l.limits.PerIPHourly {
		var oldest time.Time
		if len(state.attempts) > 0 {
			oldest = state.attempts[0]
		}
		return false, reasonPerIPHour, until(oldest, time.Hour, now)
	}

	minuteCount := 0
	var oldestMinute time.Time
	cutMinute := now.Add(-time.Minute)
	for _, t := range state.attempts {
		if t.After(cutMinute) {
			minuteCount++
			if oldestMinute.IsZero() {
				oldestMinute = t
			}
		}
	}
	if minuteCount >= l.limits.PerIPMinute {
		return false, reasonPerIPMinute, until(oldestMinute, time.Minute, now)
	}

	state.attempts = append(state.attempts, now)
	return true, "", 0
}

// ReserveModel atomically checks and reserves all model-call limits. estimatedTokens
// is charged up front so simultaneous requests cannot oversubscribe a per-IP daily
// budget. The returned finish function settles to actual usage when available and
// releases the active-stream slots; call it once when the model stream is done.
func (l *Limiter) ReserveModel(ip string, estimatedTokens int) (allowed bool, reason string, retryAfter time.Duration, finish func(actualTokens int, usageKnown bool)) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	l.sweep(now)
	today := dayKey(now)
	if today != l.day {
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

	state := l.stateFor(ip, now)
	if estimatedTokens < 0 {
		estimatedTokens = 0
	}
	if l.limits.PerIPDailyTokens <= 0 {
		return false, reasonPerIPDailyTokens, 0, nil
	}
	if state.usedTokens >= l.limits.PerIPDailyTokens || estimatedTokens > l.limits.PerIPDailyTokens-state.usedTokens {
		return false, reasonPerIPDailyTokens, untilNextDay(now), nil
	}
	if state.active >= l.limits.PerIPConcurrent {
		return false, reasonPerIPConcurrent, 0, nil
	}
	if l.globalLive >= l.limits.GlobalConcurrent {
		return false, reasonGlobalConcurrent, 0, nil
	}

	state.usedTokens += estimatedTokens
	state.active++
	l.globalTimes = append(l.globalTimes, now)
	l.globalLive++
	l.dayCount++

	var once sync.Once
	reservedDay := state.tokenDay
	finish = func(actualTokens int, usageKnown bool) {
		once.Do(func() {
			l.mu.Lock()
			defer l.mu.Unlock()
			state.active--
			l.globalLive--
			if usageKnown && state.tokenDay == reservedDay {
				if actualTokens < 0 {
					actualTokens = 0
				}
				state.usedTokens += actualTokens - estimatedTokens
				if state.usedTokens < 0 {
					state.usedTokens = 0
				}
			}
		})
	}
	return true, "", 0, finish
}

func dayKey(now time.Time) string { return now.Format("2006-01-02") }

func (l *Limiter) stateFor(ip string, now time.Time) *ipState {
	key := limitKey(ip)
	state := l.perIP[key]
	if state == nil {
		state = &ipState{}
		l.perIP[key] = state
	}
	if state.tokenDay != dayKey(now) {
		state.tokenDay = dayKey(now)
		state.usedTokens = 0
	}
	return state
}

func (l *Limiter) pruneAttempts(state *ipState, now time.Time) {
	cutHour := now.Add(-time.Hour)
	kept := state.attempts[:0]
	for _, t := range state.attempts {
		if t.After(cutHour) {
			kept = append(kept, t)
		}
	}
	state.attempts = kept
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

// sweep prunes expired attempts and removes per-IP state with no recent
// attempts, active streams, or current-day token usage. Without this, every IP
// that ever reached the service would stay in the map forever.
func (l *Limiter) sweep(now time.Time) {
	if now.Sub(l.lastSweep) < sweepInterval {
		return
	}
	l.lastSweep = now
	for key, state := range l.perIP {
		if state.tokenDay != dayKey(now) {
			state.tokenDay = dayKey(now)
			state.usedTokens = 0
		}
		l.pruneAttempts(state, now)
		if state.active > 0 {
			continue
		}
		if len(state.attempts) == 0 && state.usedTokens == 0 {
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
