package eval

// Stream is the subset of the twin stream needed to collect an answer. Keeping
// the interface local lets grading and collection stay independent of the SDK.
type Stream interface {
	Next() bool
	Text() string
	TokenUsage() (promptTokens, completionTokens, totalTokens int, known bool)
	Err() error
}

// Collect drains a stream into one answer and reports aggregate usage.
func Collect(stream Stream) (string, Usage, error) {
	var answer []byte
	for stream.Next() {
		answer = append(answer, stream.Text()...)
	}

	promptTokens, completionTokens, totalTokens, known := stream.TokenUsage()
	usage := Usage{
		PromptTokens:     promptTokens,
		CompletionTokens: completionTokens,
		TotalTokens:      totalTokens,
		Known:            known,
	}
	if err := stream.Err(); err != nil {
		return string(answer), usage, err
	}
	return string(answer), usage, nil
}
