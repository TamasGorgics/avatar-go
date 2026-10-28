package web

import (
	"context"
	"io"
	"log/slog"
	"time"

	"avatar-go/internal/config"
	"avatar-go/internal/twin"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

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
	allowed        bool
	reason         string
	retryAfter     time.Duration
	denyModel      bool
	modelReason    string
	modelRetry     time.Duration
	calls          int
	reserveCalls   int
	ip             string
	reservedTokens int
	settledTokens  int
	usageKnown     bool
	released       int
}

func (f *fakeLimiter) CheckAttempt(ip string) (bool, string, time.Duration) {
	f.calls++
	f.ip = ip
	if !f.allowed {
		return false, f.reason, f.retryAfter
	}
	return true, "", 0
}

func (f *fakeLimiter) ReserveModel(_ string, estimatedTokens int) (bool, string, time.Duration, func(int, bool)) {
	f.reserveCalls++
	f.reservedTokens = estimatedTokens
	if f.denyModel {
		reason := f.modelReason
		if reason == "" {
			reason = f.reason
		}
		retryAfter := f.modelRetry
		if retryAfter == 0 {
			retryAfter = f.retryAfter
		}
		return false, reason, retryAfter, nil
	}
	return true, "", 0, func(actualTokens int, usageKnown bool) {
		f.released++
		f.settledTokens = actualTokens
		f.usageKnown = usageKnown
	}
}

type fakeChatter struct {
	stream        twin.Stream
	err           error
	calls         int
	history       []twin.Message
	ctx           context.Context
	tokenEstimate int
}

func (f *fakeChatter) Stream(ctx context.Context, history []twin.Message) (twin.Stream, error) {
	f.calls++
	f.ctx = ctx
	f.history = append([]twin.Message(nil), history...)
	return f.stream, f.err
}

func (f *fakeChatter) EstimateTokenReservation([]twin.Message) int {
	return f.tokenEstimate
}

type fakeStream struct {
	deltas     []string
	err        error
	index      int
	closed     bool
	tokens     int
	usageKnown bool
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

func (s *fakeStream) TokenUsage() (int, bool) { return s.tokens, s.usageKnown }

func (s *fakeStream) Err() error {
	return s.err
}

func (s *fakeStream) Close() error {
	s.closed = true
	return nil
}
