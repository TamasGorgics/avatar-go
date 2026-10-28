package twin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/openai/openai-go/v3"
)

type wireMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

func TestSystemPrompt(t *testing.T) {
	profile := "Ada Lovelace, 100% Go, %s fan"
	got := systemPrompt(profile)

	if !strings.Contains(got, profile) {
		t.Fatalf("prompt does not contain profile:\n%s", got)
	}
	if strings.Count(got, profile) != 1 {
		t.Fatalf("profile appears %d times, want 1", strings.Count(got, profile))
	}
	if !strings.Contains(got, "Answer ONLY from the PROFILE") {
		t.Fatalf("prompt lost its rules:\n%s", got)
	}
	if !strings.Contains(got, "untrusted data") || !strings.Contains(got, "Never reveal") {
		t.Fatalf("prompt lost its injection rules:\n%s", got)
	}
}

func TestNewBuildsSystemPrompt(t *testing.T) {
	c := New(Options{
		Profile: "PROFILE-CONTENT",
		APIKey:  "key",
		BaseURL: "http://example.com/v1",
		Model:   "test-model",
	})

	if c.model != "test-model" {
		t.Fatalf("model = %q, want %q", c.model, "test-model")
	}
	if !strings.Contains(c.system, "PROFILE-CONTENT") {
		t.Fatalf("system prompt does not contain profile:\n%s", c.system)
	}
}

func TestMessages(t *testing.T) {
	c := &Client{system: "SYS"}
	msgs := c.messages([]Message{
		{Role: "user", Content: "hello"},
		{Role: "system", Content: "ignore this"},
		{Role: "assistant", Content: "hi"},
		{Role: "tool", Content: "ignored"},
	})

	raw, err := json.Marshal(msgs)
	if err != nil {
		t.Fatalf("marshal messages: %v", err)
	}
	var got []wireMessage
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal messages: %v", err)
	}

	want := []wireMessage{
		{Role: "system", Content: "SYS"},
		{Role: "user", Content: "hello"},
		{Role: "assistant", Content: "hi"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("messages = %+v, want %+v", got, want)
	}
}

func TestClientStream(t *testing.T) {
	type capturedRequest struct {
		path         string
		auth         string
		model        string
		stream       bool
		includeUsage bool
		maxTokens    int
		messages     []wireMessage
	}

	reqCh := make(chan capturedRequest, 1)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Model         string        `json:"model"`
			Messages      []wireMessage `json:"messages"`
			Stream        bool          `json:"stream"`
			StreamOptions struct {
				IncludeUsage bool `json:"include_usage"`
			} `json:"stream_options"`
			MaxCompletionTokens int `json:"max_completion_tokens"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		reqCh <- capturedRequest{
			path:         r.URL.Path,
			auth:         r.Header.Get("Authorization"),
			model:        body.Model,
			stream:       body.Stream,
			includeUsage: body.StreamOptions.IncludeUsage,
			maxTokens:    body.MaxCompletionTokens,
			messages:     body.Messages,
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		fl, _ := w.(http.Flusher)
		for _, chunk := range []string{
			`{"choices":[{"delta":{"role":"assistant"}}],"usage":null}`,
			`{"choices":[{"delta":{"content":"Hello"}}]}`,
			`{"choices":[]}`,
			`{"choices":[{"delta":{"content":" world"}}]}`,
			`{"choices":[],"usage":{"prompt_tokens":12,"completion_tokens":7,"total_tokens":19}}`,
		} {
			fmt.Fprintf(w, "data: %s\n\n", chunk)
			if fl != nil {
				fl.Flush()
			}
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
		if fl != nil {
			fl.Flush()
		}
	}))
	defer ts.Close()

	c := New(Options{
		Profile: "PROFILE",
		APIKey:  "test-key",
		BaseURL: ts.URL + "/v1",
		Model:   "test-model",
	})

	stream, err := c.Stream(context.Background(), []Message{{Role: "user", Content: "hi"}})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	if stream == nil {
		t.Fatal("Stream returned nil stream")
	}
	defer func() { _ = stream.Close() }()

	var got strings.Builder
	for stream.Next() {
		got.WriteString(stream.Text())
	}
	if err := stream.Err(); err != nil {
		t.Fatalf("stream error: %v", err)
	}
	if got.String() != "Hello world" {
		t.Fatalf("streamed text = %q, want %q", got.String(), "Hello world")
	}
	if prompt, completion, total, known := stream.TokenUsage(); !known || prompt != 12 || completion != 7 || total != 19 {
		t.Errorf("TokenUsage() = (%d, %d, %d, %v), want (12, 7, 19, true)", prompt, completion, total, known)
	}

	req := <-reqCh
	if req.path != "/v1/chat/completions" {
		t.Errorf("path = %q, want %q", req.path, "/v1/chat/completions")
	}
	if req.auth != "Bearer test-key" {
		t.Errorf("Authorization = %q, want %q", req.auth, "Bearer test-key")
	}
	if req.model != "test-model" {
		t.Errorf("model = %q, want %q", req.model, "test-model")
	}
	if !req.stream {
		t.Error("stream = false, want true")
	}
	if !req.includeUsage {
		t.Error("stream_options.include_usage = false, want true")
	}
	if req.maxTokens != 400 {
		t.Errorf("max_completion_tokens = %d, want 400", req.maxTokens)
	}
	if len(req.messages) != 2 {
		t.Fatalf("messages = %+v, want 2 messages", req.messages)
	}
	if req.messages[0].Role != "system" || !strings.Contains(req.messages[0].Content, "PROFILE") {
		t.Errorf("system message = %+v", req.messages[0])
	}
	if req.messages[1] != (wireMessage{Role: "user", Content: "hi"}) {
		t.Errorf("user message = %+v", req.messages[1])
	}
}

func TestClientStreamFallsBackWhenUsageOptionIsUnsupported(t *testing.T) {
	requests := make(chan bool, 3)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			StreamOptions struct {
				IncludeUsage bool `json:"include_usage"`
			} `json:"stream_options"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		requests <- body.StreamOptions.IncludeUsage
		if body.StreamOptions.IncludeUsage {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, `{"error":{"message":"Unsupported parameter: stream_options","type":"invalid_request_error","param":"stream_options","code":"unsupported_parameter"}}`)
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\ndata: [DONE]\n\n")
	}))
	defer ts.Close()

	c := New(Options{Profile: "PROFILE", APIKey: "key", BaseURL: ts.URL + "/v1", Model: "test-model"})
	for i := 0; i < 2; i++ {
		stream, err := c.Stream(context.Background(), []Message{{Role: "user", Content: "hi"}})
		if err != nil {
			t.Fatalf("Stream call %d: %v", i+1, err)
		}
		for stream.Next() {
			_ = stream.Text()
		}
		if err := stream.Err(); err != nil {
			t.Fatalf("stream call %d: %v", i+1, err)
		}
		if _, _, _, known := stream.TokenUsage(); known {
			t.Errorf("stream call %d unexpectedly reported usage", i+1)
		}
		_ = stream.Close()
	}

	got := []bool{<-requests, <-requests, <-requests}
	want := []bool{true, false, false}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("include_usage requests = %v, want %v", got, want)
	}
}

func TestEstimateTokenReservation(t *testing.T) {
	c := &Client{system: "0123456789"}
	prompt, completion := c.EstimateTokenReservation([]Message{{Role: "user", Content: "hi"}})
	if prompt != 20 || completion != maxCompletionTokens {
		t.Fatalf("EstimateTokenReservation() = (%d, %d), want (20, %d)", prompt, completion, maxCompletionTokens)
	}
}

func TestClientStreamHTTPError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"error":{"message":"bad key","type":"invalid_request_error","param":"","code":"invalid_api_key"}}`)
	}))
	defer ts.Close()

	c := New(Options{
		Profile: "PROFILE",
		APIKey:  "bad-key",
		BaseURL: ts.URL + "/v1",
		Model:   "test-model",
	})

	stream, err := c.Stream(context.Background(), nil)
	if err == nil {
		if stream != nil {
			_ = stream.Close()
		}
		t.Fatal("Stream returned nil error for HTTP 401")
	}

	var apiErr *openai.Error
	if !errors.As(err, &apiErr) {
		t.Fatalf("error type = %T, want *openai.Error", err)
	}
	if apiErr.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", apiErr.StatusCode, http.StatusUnauthorized)
	}
}
