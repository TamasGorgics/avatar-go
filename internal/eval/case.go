package eval

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

type Kind string

const (
	KindFact      Kind = "fact"
	KindRefusal   Kind = "refusal"
	KindInjection Kind = "injection"
	KindOffTopic  Kind = "off_topic"
)

type Corpus struct {
	Version int    `json:"version"`
	Cases   []Case `json:"cases"`
}

type Case struct {
	ID              string      `json:"id"`
	Kind            Kind        `json:"kind"`
	Question        string      `json:"question"`
	RequiresProfile bool        `json:"requires_profile,omitempty"`
	Expect          Expectation `json:"expect"`
}

type Expectation struct {
	// IncludeAny holds groups of alternatives. Every group must match at least
	// one term for the answer to pass.
	IncludeAny [][]string `json:"include_any"`
	Exclude    []string   `json:"exclude,omitempty"`
}

type Usage struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
	Known            bool
}

func LoadFile(path string) (Corpus, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Corpus{}, fmt.Errorf("read corpus: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var corpus Corpus
	if err := decoder.Decode(&corpus); err != nil {
		return Corpus{}, fmt.Errorf("decode corpus: %w", err)
	}
	if err := corpus.Validate(); err != nil {
		return Corpus{}, err
	}
	return corpus, nil
}

func (c Corpus) Validate() error {
	if c.Version != 1 {
		return fmt.Errorf("corpus version = %d, want 1", c.Version)
	}
	if len(c.Cases) == 0 {
		return fmt.Errorf("corpus has no cases")
	}

	seen := make(map[string]bool, len(c.Cases))
	for index, testCase := range c.Cases {
		label := testCase.ID
		if label == "" {
			label = fmt.Sprintf("case %d", index+1)
		}
		if testCase.ID == "" {
			return fmt.Errorf("%s has an empty id", label)
		}
		if seen[testCase.ID] {
			return fmt.Errorf("duplicate case id %q", testCase.ID)
		}
		seen[testCase.ID] = true

		switch testCase.Kind {
		case KindFact, KindRefusal, KindInjection, KindOffTopic:
		default:
			return fmt.Errorf("case %q has invalid kind %q", testCase.ID, testCase.Kind)
		}
		if strings.TrimSpace(testCase.Question) == "" {
			return fmt.Errorf("case %q has an empty question", testCase.ID)
		}
		if len(testCase.Expect.IncludeAny) == 0 {
			return fmt.Errorf("case %q has no include_any groups", testCase.ID)
		}
		for groupIndex, group := range testCase.Expect.IncludeAny {
			if len(group) == 0 {
				return fmt.Errorf("case %q include_any group %d is empty", testCase.ID, groupIndex+1)
			}
			for _, term := range group {
				if strings.TrimSpace(term) == "" {
					return fmt.Errorf("case %q has an empty include term", testCase.ID)
				}
			}
		}
		for _, term := range testCase.Expect.Exclude {
			if strings.TrimSpace(term) == "" {
				return fmt.Errorf("case %q has an empty exclude term", testCase.ID)
			}
		}
	}
	return nil
}

// ProfileReady reports whether the profile has content beyond the placeholder.
func ProfileReady(profile string) bool {
	trimmed := strings.TrimSpace(profile)
	return trimmed != "" && !strings.EqualFold(trimmed, "TODO")
}

// Grade returns human-readable failures. An empty slice means the answer passed.
func Grade(answer string, testCase Case) []string {
	normalized := normalize(answer)
	var failures []string
	for _, group := range testCase.Expect.IncludeAny {
		if !containsAny(normalized, group) {
			failures = append(failures, fmt.Sprintf("missing one of %q", group))
		}
	}
	for _, term := range testCase.Expect.Exclude {
		if containsTerm(normalized, normalize(term)) {
			failures = append(failures, fmt.Sprintf("contains excluded term %q", term))
		}
	}
	return failures
}

func containsAny(text string, terms []string) bool {
	for _, term := range terms {
		if containsTerm(text, normalize(term)) {
			return true
		}
	}
	return false
}

func normalize(text string) string {
	text = strings.ToLower(text)
	text = strings.ReplaceAll(text, "\u2019", "'")
	text = strings.ReplaceAll(text, "\u2018", "'")
	return strings.Join(strings.Fields(text), " ")
}

// containsTerm matches whole words so "Go" does not match "Google".
func containsTerm(text, term string) bool {
	if term == "" {
		return false
	}
	for start := 0; start+len(term) <= len(text); {
		index := strings.Index(text[start:], term)
		if index < 0 {
			return false
		}
		index += start
		end := index + len(term)
		if (index == 0 || !isWordByte(text[index-1])) && (end == len(text) || !isWordByte(text[end])) {
			return true
		}
		start = index + len(term)
	}
	return false
}

func isWordByte(value byte) bool {
	return value >= 'a' && value <= 'z' || value >= '0' && value <= '9' || value == '_' || value >= 0x80
}
