package ratelimit

import (
	"sync"
	"time"
)

type Limiter struct {
	mu       sync.Mutex
	perIP    map[string][]time.Time
	day      string
	dayCount int
	now      func() time.Time
}

func New() *Limiter {
	return NewWithClock(time.Now)
}

// NewWithClock is useful for tests that need to control the passage of time.
func NewWithClock(now func() time.Time) *Limiter {
	return &Limiter{
		perIP: map[string][]time.Time{},
		now:   now,
	}
}

// Allow enforces a per-IP hourly limit and a global daily cap (the spend guard).
func (l *Limiter) Allow(ip string, perHour, daily int) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	if today := now.Format("2006-01-02"); today != l.day {
		l.day, l.dayCount = today, 0
	}
	if l.dayCount >= daily {
		return false
	}

	cut := now.Add(-time.Hour)
	kept := l.perIP[ip][:0]
	for _, t := range l.perIP[ip] {
		if t.After(cut) {
			kept = append(kept, t)
		}
	}
	if len(kept) >= perHour {
		l.perIP[ip] = kept
		return false
	}

	l.perIP[ip] = append(kept, now)
	l.dayCount++
	return true
}
