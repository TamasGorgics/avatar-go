package main

import (
	"context"
	"errors"
	"testing"

	"avatar-go/internal/eval"
	"avatar-go/internal/twin"
)

type fakeChatter struct {
	stream  twin.Stream
	err     error
	history []twin.Message
}

func (c *fakeChatter) Stream(_ context.Context, history []twin.Message) (twin.Stream, error) {
	c.history = history
	if c.err != nil {
		return nil, c.err
	}
	return c.stream, nil
}

func (c *fakeChatter) EstimateTokenReservation([]twin.Message) (int, int) {
	return 0, 0
}

type fakeStream struct {
	chunks     []string
	index      int
	prompt     int
	completion int
	total      int
	known      bool
	err        error
	closeErr   error
	closed     bool
}

func (s *fakeStream) Next() bool {
	if s.index >= len(s.chunks) {
		return false
	}
	s.index++
	return true
}

func (s *fakeStream) Text() string {
	if s.index == 0 || s.index > len(s.chunks) {
		return ""
	}
	return s.chunks[s.index-1]
}

func (s *fakeStream) TokenUsage() (int, int, int, bool) {
	return s.prompt, s.completion, s.total, s.known
}

func (s *fakeStream) Err() error { return s.err }

func (s *fakeStream) Close() error {
	s.closed = true
	return s.closeErr
}

func TestRunCase(t *testing.T) {
	t.Run("collects answer and usage", func(t *testing.T) {
		stream := &fakeStream{
			chunks:     []string{"Hello", " world"},
			prompt:     4,
			completion: 2,
			total:      6,
			known:      true,
		}
		chat := &fakeChatter{stream: stream}

		answer, usage, err := runCase(context.Background(), chat, eval.Case{Question: "Q?"})
		if err != nil {
			t.Fatalf("runCase() error = %v", err)
		}
		if answer != "Hello world" {
			t.Fatalf("answer = %q, want %q", answer, "Hello world")
		}
		wantUsage := eval.Usage{PromptTokens: 4, CompletionTokens: 2, TotalTokens: 6, Known: true}
		if usage != wantUsage {
			t.Fatalf("usage = %+v, want %+v", usage, wantUsage)
		}
		if !stream.closed {
			t.Fatal("stream was not closed")
		}
		if len(chat.history) != 1 || chat.history[0].Role != "user" || chat.history[0].Content != "Q?" {
			t.Fatalf("history = %+v, want one user message", chat.history)
		}
	})

	t.Run("chatter error", func(t *testing.T) {
		wantErr := errors.New("upstream unavailable")
		chat := &fakeChatter{err: wantErr}
		if _, _, err := runCase(context.Background(), chat, eval.Case{}); !errors.Is(err, wantErr) {
			t.Fatalf("runCase() error = %v, want %v", err, wantErr)
		}
	})

	t.Run("stream error keeps partial answer", func(t *testing.T) {
		stream := &fakeStream{chunks: []string{"partial"}, err: errors.New("stream failed")}
		chat := &fakeChatter{stream: stream}

		answer, _, err := runCase(context.Background(), chat, eval.Case{})
		if err == nil {
			t.Fatal("runCase() error = nil, want the stream error")
		}
		if answer != "partial" {
			t.Fatalf("answer = %q, want %q", answer, "partial")
		}
		if !stream.closed {
			t.Fatal("stream was not closed after a stream error")
		}
	})

	t.Run("close error", func(t *testing.T) {
		stream := &fakeStream{chunks: []string{"answer"}, closeErr: errors.New("close failed")}
		chat := &fakeChatter{stream: stream}

		if _, _, err := runCase(context.Background(), chat, eval.Case{}); err == nil {
			t.Fatal("runCase() error = nil, want the close error")
		}
	})
}
