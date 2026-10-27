package web

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"time"

	"avatar-go/internal/twin"
)

const (
	maxHistory      = 6
	maxContentBytes = 500
	unavailableMsg  = "The twin is unavailable right now."
)

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	ip := clientIP(r, s.cfg.TrustProxy)
	if !s.lim.Allow(ip, s.cfg.PerIPHourly, s.cfg.DailyCap) {
		s.log.Warn("rate limited",
			"ip", ip,
			"per_ip_hourly", s.cfg.PerIPHourly,
			"daily_cap", s.cfg.DailyCap,
		)
		http.Error(w, "Rate limit reached, please try again later.", http.StatusTooManyRequests)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	var in struct {
		History []chatMessage `json:"history"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || len(in.History) == 0 {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	// Overall cap on the request, including the streamed body.
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()

	history := sanitize(in.History)
	s.log.Debug("chat request", "ip", ip, "messages", len(history))

	stream, err := s.chat.Stream(ctx, history)
	if err != nil {
		s.log.Error("upstream error", "err", err, "ip", ip)
		http.Error(w, unavailableMsg, http.StatusBadGateway)
		return
	}
	defer func() { _ = stream.Close() }()

	s.writeStream(w, stream)
}

// sanitize keeps the last messages, only user/assistant roles, capped length.
// Visitors can never inject a "system" message.
func sanitize(history []chatMessage) []twin.Message {
	if len(history) > maxHistory {
		history = history[len(history)-maxHistory:]
	}

	msgs := make([]twin.Message, 0, len(history))
	for _, m := range history {
		if m.Role != "user" && m.Role != "assistant" {
			continue
		}
		content := m.Content
		if len(content) > maxContentBytes {
			content = content[:maxContentBytes]
		}
		msgs = append(msgs, twin.Message{Role: m.Role, Content: content})
	}
	return msgs
}

func (s *Server) writeStream(w http.ResponseWriter, stream twin.Stream) {
	fl, _ := w.(http.Flusher)
	wrote := false
	for stream.Next() {
		delta := stream.Text()
		if delta == "" {
			continue
		}
		if !wrote {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.Header().Set("Cache-Control", "no-cache")
			w.Header().Set("X-Accel-Buffering", "no")
			wrote = true
		}
		if _, err := w.Write([]byte(delta)); err != nil {
			s.log.Debug("write response", "err", err)
			return
		}
		if fl != nil {
			fl.Flush()
		}
	}
	if err := stream.Err(); err != nil {
		s.log.Error("upstream stream error", "err", err)
		if !wrote {
			http.Error(w, unavailableMsg, http.StatusBadGateway)
		}
	}
}

func clientIP(r *http.Request, trustProxy bool) string {
	if trustProxy {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			return strings.TrimSpace(strings.Split(xff, ",")[0])
		}
	}
	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	return host
}
