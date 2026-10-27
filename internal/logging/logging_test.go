package logging

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestJSONLog(t *testing.T) {
	var buf bytes.Buffer
	logger := New(Options{Writer: &buf, Format: FormatJSON, Level: "debug"})

	logger.Info("hello", "answer", 42, "ok", true, "took", 1500*time.Millisecond)

	var got map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &got); err != nil {
		t.Fatalf("unmarshal log: %v\n%s", err, buf.String())
	}
	if got["level"] != "INFO" {
		t.Errorf("level = %v, want INFO", got["level"])
	}
	if got["msg"] != "hello" {
		t.Errorf("msg = %v, want hello", got["msg"])
	}
	if got["answer"] != float64(42) {
		t.Errorf("answer = %v, want 42", got["answer"])
	}
	if got["ok"] != true {
		t.Errorf("ok = %v, want true", got["ok"])
	}
	if got["took"] != float64(1500*time.Millisecond) {
		t.Errorf("took = %v, want nanoseconds", got["took"])
	}
	src, ok := got["source"].(map[string]any)
	if !ok || src["file"] == "" || src["line"] == nil {
		t.Errorf("source = %v, want file and line", got["source"])
	}
}

func TestTextLog(t *testing.T) {
	var buf bytes.Buffer
	logger := New(Options{Writer: &buf, Format: FormatText, Level: "info"})

	logger.Info("hello world", "answer", 42, "err", errors.New("boom"), "took", time.Second)

	out := buf.String()
	for _, want := range []string{
		"INFO ",
		`msg="hello world"`,
		"answer=42",
		`err=boom`,
		"took=1s",
		"source=logging_test.go:",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("log output missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "\x1b[") {
		t.Errorf("non-terminal text log contains colors:\n%s", out)
	}
}

func TestAutoFormatUsesJSONWithoutTerminal(t *testing.T) {
	for _, format := range []string{FormatAuto, "", "unknown", " JSON "} {
		t.Run(format, func(t *testing.T) {
			var buf bytes.Buffer
			New(Options{Writer: &buf, Format: format}).Info("auto")

			var got map[string]any
			if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &got); err != nil {
				t.Fatalf("format %q did not produce JSON: %v\n%s", format, err, buf.String())
			}
			if got["msg"] != "auto" {
				t.Errorf("msg = %v, want auto", got["msg"])
			}
		})
	}
}

func TestLevelFiltering(t *testing.T) {
	var buf bytes.Buffer
	logger := New(Options{Writer: &buf, Format: FormatJSON, Level: "warn"})

	logger.Info("hidden")
	logger.Warn("shown")

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 1 {
		t.Fatalf("got %d log lines, want 1:\n%s", len(lines), buf.String())
	}
	if !strings.Contains(lines[0], `"msg":"shown"`) || !strings.Contains(lines[0], `"level":"WARN"`) {
		t.Fatalf("unexpected log line: %s", lines[0])
	}
}

func TestInvalidLevelFallsBackToInfo(t *testing.T) {
	var buf bytes.Buffer
	logger := New(Options{Writer: &buf, Format: FormatJSON, Level: "verbose"})

	logger.Debug("hidden")
	logger.Info("shown")

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 1 || !strings.Contains(lines[0], `"msg":"shown"`) {
		t.Fatalf("invalid level did not fall back to info:\n%s", buf.String())
	}
}

func TestTextHandlerColor(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(newTextHandler(&buf, slog.LevelInfo, true))

	logger.Info("colored", "n", 1)

	out := buf.String()
	if !strings.Contains(out, ansiGreen+"INFO "+ansiReset) {
		t.Errorf("INFO is not colored:\n%q", out)
	}
	if !strings.Contains(out, ansiDim) {
		t.Errorf("metadata is not dimmed:\n%q", out)
	}
}

func TestTextHandlerAttrsAndGroups(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(newTextHandler(&buf, slog.LevelDebug, false)).
		With("service", "twin").
		WithGroup("req")

	logger.Info("request", "method", "POST", slog.Group("http", "status", 200))

	out := buf.String()
	for _, want := range []string{
		"service=twin",
		"req.method=POST",
		"req.http=[status=200]",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("log output missing %q:\n%s", want, out)
		}
	}
}

func TestTextHandlerValues(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(newTextHandler(&buf, slog.LevelDebug, false))

	logger.Info("values",
		"string", "hello world",
		"uint", uint64(8),
		"float", 1.25,
		"bool", true,
		"when", time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
		"any", []int{1, 2},
		slog.Group("group", "k", "v"),
	)

	out := buf.String()
	for _, want := range []string{
		`string="hello world"`,
		"uint=8",
		"float=1.25",
		"bool=true",
		"when=2026-01-02T03:04:05Z",
		`any="[1 2]"`,
		"group=[k=v]",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("log output missing %q:\n%s", want, out)
		}
	}
}

func TestLevelColor(t *testing.T) {
	tests := []struct {
		level slog.Level
		want  string
	}{
		{level: slog.LevelDebug, want: ansiCyan},
		{level: slog.LevelInfo, want: ansiGreen},
		{level: slog.LevelWarn, want: ansiYellow},
		{level: slog.LevelError, want: ansiRed},
		{level: slog.LevelError + 4, want: ansiRed},
	}
	for _, tc := range tests {
		if got := levelColor(tc.level); got != tc.want {
			t.Errorf("levelColor(%s) = %q, want %q", tc.level, got, tc.want)
		}
	}
}
