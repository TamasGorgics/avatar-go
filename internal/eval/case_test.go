package eval

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadFileCorpus(t *testing.T) {
	corpus, err := LoadFile("testdata/cases.json")
	if err != nil {
		t.Fatalf("load corpus: %v", err)
	}

	kinds := make(map[Kind]int)
	requiresProfile := 0
	for _, testCase := range corpus.Cases {
		kinds[testCase.Kind]++
		if testCase.RequiresProfile {
			requiresProfile++
		}
		if testCase.Kind == KindFact && !testCase.RequiresProfile {
			t.Errorf("fact case %q must require a ready profile", testCase.ID)
		}
	}

	for _, kind := range []Kind{KindFact, KindRefusal, KindInjection, KindOffTopic} {
		if kinds[kind] == 0 {
			t.Errorf("corpus has no %q cases", kind)
		}
	}
	if requiresProfile == 0 {
		t.Error("corpus has no cases marked requires_profile")
	}
	if len(corpus.Cases) < 10 {
		t.Errorf("corpus has only %d cases", len(corpus.Cases))
	}
}

func TestLoadFileRejectsUnknownFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cases.json")
	data := `{
		"version": 1,
		"cases": [{
			"id": "case",
			"kind": "refusal",
			"question": "Q?",
			"expect": {"include_any": [["answer"]]}
		}],
		"unexpected": true
	}`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatalf("write corpus: %v", err)
	}

	if _, err := LoadFile(path); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("LoadFile error = %v, want an unknown field error", err)
	}
}

func TestValidate(t *testing.T) {
	valid := func() Corpus {
		return Corpus{
			Version: 1,
			Cases: []Case{{
				ID:       "case",
				Kind:     KindRefusal,
				Question: "Q?",
				Expect:   Expectation{IncludeAny: [][]string{{"answer"}}},
			}},
		}
	}

	tests := []struct {
		name   string
		mutate func(*Corpus)
		want   string
	}{
		{
			name:   "valid",
			mutate: func(*Corpus) {},
		},
		{
			name:   "bad version",
			mutate: func(c *Corpus) { c.Version = 2 },
			want:   "version = 2",
		},
		{
			name:   "no cases",
			mutate: func(c *Corpus) { c.Cases = nil },
			want:   "no cases",
		},
		{
			name:   "empty id",
			mutate: func(c *Corpus) { c.Cases[0].ID = "" },
			want:   "empty id",
		},
		{
			name:   "duplicate id",
			mutate: func(c *Corpus) { c.Cases = append(c.Cases, c.Cases[0]) },
			want:   `duplicate case id "case"`,
		},
		{
			name:   "invalid kind",
			mutate: func(c *Corpus) { c.Cases[0].Kind = "other" },
			want:   `invalid kind "other"`,
		},
		{
			name:   "empty question",
			mutate: func(c *Corpus) { c.Cases[0].Question = "  " },
			want:   "empty question",
		},
		{
			name:   "no include groups",
			mutate: func(c *Corpus) { c.Cases[0].Expect.IncludeAny = nil },
			want:   "no include_any groups",
		},
		{
			name:   "empty include group",
			mutate: func(c *Corpus) { c.Cases[0].Expect.IncludeAny = [][]string{{}} },
			want:   "group 1 is empty",
		},
		{
			name:   "empty include term",
			mutate: func(c *Corpus) { c.Cases[0].Expect.IncludeAny = [][]string{{" "}} },
			want:   "empty include term",
		},
		{
			name:   "empty exclude term",
			mutate: func(c *Corpus) { c.Cases[0].Expect.Exclude = []string{" "} },
			want:   "empty exclude term",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			corpus := valid()
			tt.mutate(&corpus)
			err := corpus.Validate()
			if tt.want == "" {
				if err != nil {
					t.Fatalf("Validate() error = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Validate() error = %v, want it to contain %q", err, tt.want)
			}
		})
	}
}

func TestProfileReady(t *testing.T) {
	tests := []struct {
		name    string
		profile string
		want    bool
	}{
		{name: "empty", profile: "", want: false},
		{name: "whitespace", profile: " \n\t ", want: false},
		{name: "placeholder", profile: "TODO", want: false},
		{name: "placeholder with whitespace", profile: "\n todo \n", want: false},
		{name: "content", profile: "Tamas builds Go services.", want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ProfileReady(tt.profile); got != tt.want {
				t.Fatalf("ProfileReady(%q) = %v, want %v", tt.profile, got, tt.want)
			}
		})
	}
}

func TestGrade(t *testing.T) {
	tests := []struct {
		name     string
		answer   string
		expect   Expectation
		want     int
		contains string
	}{
		{
			name:   "all groups match",
			answer: "I build Go services.",
			expect: Expectation{IncludeAny: [][]string{{"go"}, {"services"}}},
		},
		{
			name:   "case insensitive",
			answer: "I BUILD GO SERVICES.",
			expect: Expectation{IncludeAny: [][]string{{"go"}, {"services"}}},
		},
		{
			name:   "alternative term",
			answer: "I write Golang services.",
			expect: Expectation{IncludeAny: [][]string{{"go", "golang"}, {"services"}}},
		},
		{
			name:     "missing group",
			answer:   "I build services.",
			expect:   Expectation{IncludeAny: [][]string{{"go"}, {"services"}}},
			want:     1,
			contains: "missing one of",
		},
		{
			name:     "whole word only",
			answer:   "I build Google services.",
			expect:   Expectation{IncludeAny: [][]string{{"go"}}},
			want:     1,
			contains: "missing one of",
		},
		{
			name:     "underscore is part of a word",
			answer:   "I use go_version.",
			expect:   Expectation{IncludeAny: [][]string{{"go"}}},
			want:     1,
			contains: "missing one of",
		},
		{
			name:   "punctuation boundary",
			answer: "I build Go.",
			expect: Expectation{IncludeAny: [][]string{{"go"}}},
		},
		{
			name:   "curly apostrophe",
			answer: "I don’t know.",
			expect: Expectation{IncludeAny: [][]string{{"i don't know"}}},
		},
		{
			name:   "collapsed whitespace",
			answer: "I   don't\nknow.",
			expect: Expectation{IncludeAny: [][]string{{"i don't know"}}},
		},
		{
			name:     "excluded term",
			answer:   "I build Go services. Email me.",
			expect:   Expectation{IncludeAny: [][]string{{"go"}}, Exclude: []string{"email"}},
			want:     1,
			contains: "contains excluded term",
		},
		{
			name:   "exclude is whole word",
			answer: "I use Google.",
			expect: Expectation{IncludeAny: [][]string{{"google"}}, Exclude: []string{"go"}},
		},
		{
			name:     "excluded exact word",
			answer:   "I use Go.",
			expect:   Expectation{IncludeAny: [][]string{{"go"}}, Exclude: []string{"go"}},
			want:     1,
			contains: "contains excluded term",
		},
		{
			name:     "empty answer",
			answer:   "",
			expect:   Expectation{IncludeAny: [][]string{{"go"}}},
			want:     1,
			contains: "missing one of",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			failures := Grade(tt.answer, Case{Expect: tt.expect})
			if len(failures) != tt.want {
				t.Fatalf("Grade() failures = %q, want %d", failures, tt.want)
			}
			if tt.contains != "" && !strings.Contains(strings.Join(failures, "\n"), tt.contains) {
				t.Fatalf("Grade() failures = %q, want a failure containing %q", failures, tt.contains)
			}
		})
	}
}

type fakeStream struct {
	chunks     []string
	index      int
	prompt     int
	completion int
	total      int
	known      bool
	err        error
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

func TestCollect(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		stream := &fakeStream{
			chunks:     []string{"Hello", " ", "world"},
			prompt:     12,
			completion: 3,
			total:      15,
			known:      true,
		}
		answer, usage, err := Collect(stream)
		if err != nil {
			t.Fatalf("Collect() error = %v", err)
		}
		if answer != "Hello world" {
			t.Fatalf("answer = %q, want %q", answer, "Hello world")
		}
		want := Usage{PromptTokens: 12, CompletionTokens: 3, TotalTokens: 15, Known: true}
		if usage != want {
			t.Fatalf("usage = %+v, want %+v", usage, want)
		}
	})

	t.Run("stream error keeps partial answer", func(t *testing.T) {
		stream := &fakeStream{chunks: []string{"partial"}, err: os.ErrDeadlineExceeded}
		answer, _, err := Collect(stream)
		if err == nil {
			t.Fatal("Collect() error = nil, want the stream error")
		}
		if answer != "partial" {
			t.Fatalf("answer = %q, want %q", answer, "partial")
		}
	})

	t.Run("usage absent", func(t *testing.T) {
		stream := &fakeStream{chunks: []string{"answer"}}
		answer, usage, err := Collect(stream)
		if err != nil {
			t.Fatalf("Collect() error = %v", err)
		}
		if answer != "answer" {
			t.Fatalf("answer = %q, want %q", answer, "answer")
		}
		if usage.Known {
			t.Fatalf("usage = %+v, want unknown", usage)
		}
	})
}
