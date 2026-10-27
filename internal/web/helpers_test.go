package web

import (
	"context"

	"avatar-go/internal/config"
	"avatar-go/internal/twin"
)

func testConfig() config.Config {
	return config.Config{
		Addr:        ":0",
		LLMBaseURL:  "http://example.com/v1",
		APIKey:      "test-key",
		Model:       "test-model",
		DailyCap:    100,
		PerIPHourly: 10,
	}
}

type fakeLimiter struct {
	allowed bool
	calls   int
	ip      string
	perHour int
	daily   int
}

func (f *fakeLimiter) Allow(ip string, perHour, daily int) bool {
	f.calls++
	f.ip = ip
	f.perHour = perHour
	f.daily = daily
	return f.allowed
}

type fakeChatter struct {
	stream  twin.Stream
	err     error
	calls   int
	history []twin.Message
	ctx     context.Context
}

func (f *fakeChatter) Stream(ctx context.Context, history []twin.Message) (twin.Stream, error) {
	f.calls++
	f.ctx = ctx
	f.history = append([]twin.Message(nil), history...)
	return f.stream, f.err
}

type fakeStream struct {
	deltas []string
	err    error
	index  int
	closed bool
}

func (s *fakeStream) Next() bool {
	if s.index >= len(s.deltas) {
		return false
	}
	s.index++
	return true
}

func (s *fakeStream) Text() string {
	if s.index == 0 || s.index > len(s.deltas) {
		return ""
	}
	return s.deltas[s.index-1]
}

func (s *fakeStream) Err() error {
	return s.err
}

func (s *fakeStream) Close() error {
	s.closed = true
	return nil
}
