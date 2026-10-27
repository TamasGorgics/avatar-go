// Package logging builds the application's slog logger.
package logging

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
)

const (
	// FormatAuto selects text on a terminal and JSON elsewhere.
	FormatAuto = "auto"
	// FormatText forces the development text handler.
	FormatText = "text"
	// FormatJSON forces newline-delimited JSON.
	FormatJSON = "json"

	timeFormat = "2006-01-02T15:04:05.000Z07:00"
)

const (
	ansiReset  = "\x1b[0m"
	ansiDim    = "\x1b[2m"
	ansiRed    = "\x1b[31m"
	ansiGreen  = "\x1b[32m"
	ansiYellow = "\x1b[33m"
	ansiCyan   = "\x1b[36m"
)

// Options configures New.
type Options struct {
	Writer io.Writer
	Format string
	Level  string
}

// New returns a logger that writes to opts.Writer (stderr by default).
// Format "auto" picks colored text when the writer is a terminal and JSON
// otherwise, so containers, pipes and CI get structured logs by default.
func New(opts Options) *slog.Logger {
	w := opts.Writer
	if w == nil {
		w = os.Stderr
	}
	level := parseLevel(opts.Level)

	if resolveFormat(opts.Format, w) == FormatJSON {
		return slog.New(slog.NewJSONHandler(w, &slog.HandlerOptions{
			AddSource: true,
			Level:     level,
		}))
	}
	return slog.New(newTextHandler(w, level, useColor(w)))
}

func resolveFormat(format string, w io.Writer) string {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case FormatJSON:
		return FormatJSON
	case FormatText:
		return FormatText
	default:
		if isTerminal(w) {
			return FormatText
		}
		return FormatJSON
	}
}

func parseLevel(level string) slog.Level {
	var l slog.Level
	if err := l.UnmarshalText([]byte(strings.TrimSpace(level))); err != nil {
		return slog.LevelInfo
	}
	return l
}

func isTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	info, err := f.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

func useColor(w io.Writer) bool {
	if os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb" {
		return false
	}
	return isTerminal(w)
}

// textHandler is a small logfmt-style handler for local development. It adds
// colors and a shortened source location, and writes one line per record.
type textHandler struct {
	w      io.Writer
	level  slog.Level
	color  bool
	groups []string
	attrs  []groupedAttr
}

type groupedAttr struct {
	groups []string
	attr   slog.Attr
}

func newTextHandler(w io.Writer, level slog.Level, color bool) *textHandler {
	return &textHandler{w: w, level: level, color: color}
}

func (h *textHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level
}

func (h *textHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	if len(attrs) == 0 {
		return h
	}
	h2 := *h
	h2.attrs = slices.Clone(h.attrs)
	groups := slices.Clone(h.groups)
	for _, a := range attrs {
		h2.attrs = append(h2.attrs, groupedAttr{groups: groups, attr: a})
	}
	return &h2
}

func (h *textHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	h2 := *h
	h2.groups = append(slices.Clone(h.groups), name)
	return &h2
}

func (h *textHandler) Handle(_ context.Context, r slog.Record) error {
	var b strings.Builder
	b.Grow(256)

	h.writePart(&b, ansiDim, r.Time.Format(timeFormat))
	b.WriteByte(' ')

	level := r.Level.String()
	if len(level) < 5 {
		level += strings.Repeat(" ", 5-len(level))
	}
	h.writePart(&b, levelColor(r.Level), level)
	b.WriteByte(' ')

	if src := source(r.PC); src != "" {
		h.writePart(&b, ansiDim, "source="+src)
		b.WriteByte(' ')
	}

	b.WriteString("msg=")
	b.WriteString(quote(r.Message))

	for _, a := range h.attrs {
		appendAttr(&b, a.groups, a.attr)
	}
	r.Attrs(func(a slog.Attr) bool {
		appendAttr(&b, h.groups, a)
		return true
	})

	b.WriteByte('\n')
	_, err := io.WriteString(h.w, b.String())
	return err
}

func (h *textHandler) writePart(b *strings.Builder, color, s string) {
	if !h.color {
		b.WriteString(s)
		return
	}
	b.WriteString(color)
	b.WriteString(s)
	b.WriteString(ansiReset)
}

func source(pc uintptr) string {
	if pc == 0 {
		return ""
	}
	frame, _ := runtime.CallersFrames([]uintptr{pc}).Next()
	if frame.File == "" {
		return ""
	}
	return filepath.Base(frame.File) + ":" + strconv.Itoa(frame.Line)
}

func levelColor(level slog.Level) string {
	switch {
	case level >= slog.LevelError:
		return ansiRed
	case level >= slog.LevelWarn:
		return ansiYellow
	case level >= slog.LevelInfo:
		return ansiGreen
	default:
		return ansiCyan
	}
}

func appendAttr(b *strings.Builder, groups []string, a slog.Attr) {
	if a.Key == "" {
		return
	}
	a.Value = a.Value.Resolve()

	b.WriteByte(' ')
	b.WriteString(quote(joinKey(groups, a.Key)))
	b.WriteByte('=')

	if a.Value.Kind() == slog.KindGroup {
		appendGroup(b, a.Value.Group())
		return
	}
	appendValue(b, a.Value)
}

func joinKey(groups []string, key string) string {
	if len(groups) == 0 {
		return key
	}
	return strings.Join(groups, ".") + "." + key
}

func appendGroup(b *strings.Builder, attrs []slog.Attr) {
	b.WriteByte('[')
	wrote := false
	for _, a := range attrs {
		if a.Key == "" {
			continue
		}
		a.Value = a.Value.Resolve()
		if wrote {
			b.WriteByte(' ')
		}
		wrote = true
		b.WriteString(quote(a.Key))
		b.WriteByte('=')
		if a.Value.Kind() == slog.KindGroup {
			appendGroup(b, a.Value.Group())
		} else {
			appendValue(b, a.Value)
		}
	}
	b.WriteByte(']')
}

func appendValue(b *strings.Builder, v slog.Value) {
	v = v.Resolve()
	switch v.Kind() {
	case slog.KindString:
		b.WriteString(quote(v.String()))
	case slog.KindInt64:
		b.WriteString(strconv.FormatInt(v.Int64(), 10))
	case slog.KindUint64:
		b.WriteString(strconv.FormatUint(v.Uint64(), 10))
	case slog.KindFloat64:
		b.WriteString(strconv.FormatFloat(v.Float64(), 'g', -1, 64))
	case slog.KindBool:
		b.WriteString(strconv.FormatBool(v.Bool()))
	case slog.KindDuration:
		b.WriteString(v.Duration().String())
	case slog.KindTime:
		b.WriteString(v.Time().Format(time.RFC3339Nano))
	case slog.KindGroup:
		appendGroup(b, v.Group())
	case slog.KindAny:
		if err, ok := v.Any().(error); ok {
			b.WriteString(quote(err.Error()))
			return
		}
		b.WriteString(quote(fmt.Sprint(v.Any())))
	default:
		b.WriteString(quote(fmt.Sprint(v.Any())))
	}
}

func quote(s string) string {
	if s == "" {
		return `""`
	}
	for _, r := range s {
		if r <= ' ' || r == '=' || r == '"' || r == '\\' || !unicode.IsPrint(r) {
			return strconv.Quote(s)
		}
	}
	return s
}
