package twin

import (
	"context"

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
}

// Stream yields the assistant's text one delta at a time.
type Stream interface {
	Next() bool
	Text() string
	Err() error
	Close() error
}

type Client struct {
	ai     openai.Client
	model  string
	system string
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
	stream := c.ai.Chat.Completions.NewStreaming(ctx, openai.ChatCompletionNewParams{
		Model:               c.model,
		Messages:            c.messages(history),
		MaxCompletionTokens: openai.Int(400), // newer OpenAI models reject max_tokens
	})
	if err := stream.Err(); err != nil {
		_ = stream.Close()
		return nil, err
	}
	return &openAIStream{stream: stream}, nil
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
	stream *ssestream.Stream[openai.ChatCompletionChunk]
}

func (s *openAIStream) Next() bool { return s.stream.Next() }

func (s *openAIStream) Text() string {
	chunk := s.stream.Current()
	if len(chunk.Choices) == 0 {
		return ""
	}
	return chunk.Choices[0].Delta.Content
}

func (s *openAIStream) Err() error { return s.stream.Err() }

func (s *openAIStream) Close() error { return s.stream.Close() }
