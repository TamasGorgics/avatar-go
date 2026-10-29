package web

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"
)

type screeningCorpus struct {
	Version int             `json:"version"`
	Cases   []screeningCase `json:"cases"`
}

type screeningCase struct {
	ID       string   `json:"id"`
	Messages []string `json:"messages"`
	Want     string   `json:"want"`
	Reason   string   `json:"reason"`
}

// TestScreeningCorpus is the regression guard for injection screening. It runs
// in normal CI without a model and covers both attacks and benign questions
// that must not be rejected. When a false positive is reported in production,
// add it here as an "allow" case before tuning the rules.
func TestScreeningCorpus(t *testing.T) {
	data, err := os.ReadFile("testdata/screening.json")
	if err != nil {
		t.Fatalf("read screening corpus: %v", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var corpus screeningCorpus
	if err := decoder.Decode(&corpus); err != nil {
		t.Fatalf("decode screening corpus: %v", err)
	}
	if corpus.Version != 1 {
		t.Fatalf("corpus version = %d, want 1", corpus.Version)
	}

	seen := make(map[string]bool, len(corpus.Cases))
	wants := make(map[string]int)
	reasons := make(map[string]bool)
	for _, tc := range corpus.Cases {
		if tc.ID == "" {
			t.Fatal("case has an empty id")
		}
		if seen[tc.ID] {
			t.Fatalf("duplicate case id %q", tc.ID)
		}
		seen[tc.ID] = true
		if len(tc.Messages) == 0 {
			t.Fatalf("case %q has no messages", tc.ID)
		}
		switch tc.Want {
		case "allow", "block":
			wants[tc.Want]++
		default:
			t.Fatalf("case %q has invalid want %q", tc.ID, tc.Want)
		}
		if tc.Want == "block" {
			if tc.Reason == "" {
				t.Fatalf("block case %q has no expected reason", tc.ID)
			}
			reasons[tc.Reason] = true
		}

		t.Run(tc.ID, func(t *testing.T) {
			history := make([]chatMessage, 0, len(tc.Messages))
			for _, message := range tc.Messages {
				history = append(history, chatMessage{Role: "user", Content: message})
			}

			got := screenHistory(sanitize(history))
			switch tc.Want {
			case "allow":
				if got != "" {
					t.Fatalf("benign message was rejected by %q", got)
				}
			case "block":
				if got == "" {
					t.Fatal("injection message was allowed")
				}
				if got != tc.Reason {
					t.Fatalf("reason = %q, want %q", got, tc.Reason)
				}
			}
		})
	}

	if wants["block"] < 10 || wants["allow"] < 10 {
		t.Fatalf("corpus coverage too small: %d block, %d allow", wants["block"], wants["allow"])
	}
	for _, reason := range []string{"instruction_override", "prompt_reveal", "instruction_probe", "persona_override"} {
		if !reasons[reason] {
			t.Errorf("corpus has no block case for reason %q", reason)
		}
	}
}
