package twin

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/packages/ssestream"
)

type Options struct {
	Profile string
	APIKey  string
	BaseURL string
	Model   string
}

type Message struct {
	Role    string
	Content string
}

// Chatter is the model dependency of the HTTP layer.
type Chatter interface {
	Stream(ctx context.Context, history []Message) (Stream, error)
	EstimateTokenReservation(history []Message) int
}

// Stream yields assistant text and optional aggregate token usage.
type Stream interface {
	Next() bool
	Text() string
	TokenUsage() (tokens int, known bool)
	Err() error
	Close() error
}

const (
	maxCompletionTokens   = 400
	bytesPerTokenEstimate = 3
	messageTokenOverhead  = 8
)

type Client struct {
	ai                  openai.Client
	model               string
	system              string
	usageOptionRejected atomic.Bool
}

func New(opts Options) *Client {
	return &Client{
		ai: openai.NewClient(
			option.WithAPIKey(opts.APIKey),
			option.WithBaseURL(opts.BaseURL),
		),
		model:  opts.Model,
		system: systemPrompt(opts.Profile),
	}
}

func (c *Client) Stream(ctx context.Context, history []Message) (Stream, error) {
	includeUsage := !c.usageOptionRejected.Load()
	stream := c.newStreaming(ctx, history, includeUsage)
	if err := stream.Err(); err != nil {
		_ = stream.Close()
		if !includeUsage || !usageOptionUnsupported(err) {
			return nil, err
		}

		// Some OpenAI-compatible endpoints reject stream_options. Remember that
		// capability and retry without usage metadata to preserve compatibility.
		c.usageOptionRejected.Store(true)
		stream = c.newStreaming(ctx, history, false)
		if err := stream.Err(); err != nil {
			_ = stream.Close()
			return nil, err
		}
	}
	return &openAIStream{stream: stream}, nil
}

func (c *Client) newStreaming(ctx context.Context, history []Message, includeUsage bool) *ssestream.Stream[openai.ChatCompletionChunk] {
	params := openai.ChatCompletionNewParams{
		Model:               c.model,
		Messages:            c.messages(history),
		MaxCompletionTokens: openai.Int(maxCompletionTokens),
		ReasoningEffort:     openai.ReasoningEffortNone,
	}
	if includeUsage {
		params.StreamOptions = openai.ChatCompletionStreamOptionsParam{
			IncludeUsage: openai.Bool(true),
		}
	}
	return c.ai.Chat.Completions.NewStreaming(ctx, params)
}

func usageOptionUnsupported(err error) bool {
	var apiErr *openai.Error
	if !errors.As(err, &apiErr) || (apiErr.StatusCode != 400 && apiErr.StatusCode != 422) {
		return false
	}

	text := strings.ToLower(apiErr.Message + " " + apiErr.Param + " " + apiErr.Code)
	if strings.Contains(strings.ToLower(apiErr.Param), "stream_options") {
		return true
	}
	if !strings.Contains(text, "stream_options") && !strings.Contains(text, "include_usage") {
		return false
	}
	for _, marker := range []string{
		"unsupported", "not support", "not accepted", "unknown", "unrecognized", "not allowed",
		"unexpected", "extra field", "extra input", "additional propert",
	} {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}

// EstimateTokenReservation provides a pre-call estimate for the daily per-IP
// budget. It includes the system prompt, message content, chat framing overhead,
// and the maximum completion allowance. Provider-reported usage replaces this
// estimate when available.
func (c *Client) EstimateTokenReservation(history []Message) int {
	promptBytes := len(c.system)
	for _, message := range history {
		promptBytes += len(message.Content)
	}
	inputTokens := (promptBytes + bytesPerTokenEstimate - 1) / bytesPerTokenEstimate
	inputTokens += (len(history) + 1) * messageTokenOverhead
	return inputTokens + maxCompletionTokens
}

func (c *Client) messages(history []Message) []openai.ChatCompletionMessageParamUnion {
	msgs := make([]openai.ChatCompletionMessageParamUnion, 0, len(history)+1)
	msgs = append(msgs, openai.SystemMessage(c.system))
	for _, m := range history {
		switch m.Role {
		case "user":
			msgs = append(msgs, openai.UserMessage(m.Content))
		case "assistant":
			msgs = append(msgs, openai.AssistantMessage(m.Content))
		}
	}
	return msgs
}

type openAIStream struct {
	stream     *ssestream.Stream[openai.ChatCompletionChunk]
	usage      int
	usageKnown bool
}

func (s *openAIStream) Next() bool {
	if !s.stream.Next() {
		return false
	}

	chunk := s.stream.Current()
	usage := chunk.Usage
	if usage.JSON.TotalTokens.Valid() {
		s.usage = int(usage.TotalTokens)
		s.usageKnown = true
	} else if usage.JSON.PromptTokens.Valid() || usage.JSON.CompletionTokens.Valid() {
		s.usage = int(usage.PromptTokens + usage.CompletionTokens)
		s.usageKnown = true
	}
	return true
}

func (s *openAIStream) Text() string {
	chunk := s.stream.Current()
	if len(chunk.Choices) == 0 {
		return ""
	}
	return chunk.Choices[0].Delta.Content
}

func (s *openAIStream) TokenUsage() (int, bool) { return s.usage, s.usageKnown }

func (s *openAIStream) Err() error { return s.stream.Err() }

func (s *openAIStream) Close() error { return s.stream.Close() }
