package web

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"avatar-go/internal/twin"
)

const (
	maxHistory      = 6
	maxContentBytes = 500
	maxBodyBytes    = 16 << 10
	unavailableMsg  = "The twin is unavailable right now."
	rateLimitedMsg  = "Too many questions right now. Please try again later."
	busyMsg         = "Another answer is still in progress. Please wait for it to finish."
	rejectedMsg     = "That message can't be processed."
)

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")

	ip := clientIP(r, s.cfg.TrustProxy)
	allowed, reason, retryAfter := s.lim.CheckAttempt(ip)
	if !allowed {
		s.writeRateLimit(w, ip, reason, retryAfter)
		return
	}

	if !isJSON(r.Header.Get("Content-Type")) {
		http.Error(w, "unsupported media type", http.StatusUnsupportedMediaType)
		return
	}
	if !sameOrigin(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	var in struct {
		History []chatMessage `json:"history"`
	}
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&in); err != nil {
		if isMaxBytesError(err) {
			http.Error(w, "request too large", http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if isMaxBytesError(err) {
			http.Error(w, "request too large", http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if len(in.History) == 0 {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	history := sanitize(in.History)
	if len(history) == 0 {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	last := history[len(history)-1]
	if last.Role != "user" || strings.TrimSpace(last.Content) == "" {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if reason := screenHistory(history); reason != "" {
		s.log.Warn("chat rejected", "ip", ip, "reason", reason)
		http.Error(w, rejectedMsg, http.StatusBadRequest)
		return
	}

	allowed, reason, retryAfter, finish := s.lim.ReserveModel(ip, s.chat.EstimateTokenReservation(history))
	if !allowed {
		s.writeRateLimit(w, ip, reason, retryAfter)
		return
	}

	var stream twin.Stream
	var actualTokens int
	var usageKnown bool
	if finish != nil {
		defer func() {
			if stream != nil {
				actualTokens, usageKnown = stream.TokenUsage()
			}
			finish(actualTokens, usageKnown)
		}()
	}

	// Overall cap on the request, including the streamed body.
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()

	s.log.Debug("chat request", "ip", ip, "messages", len(history))

	var err error
	stream, err = s.chat.Stream(ctx, history)
	if err != nil {
		s.log.Error("upstream error", "err", err, "ip", ip)
		http.Error(w, unavailableMsg, http.StatusBadGateway)
		return
	}
	if stream == nil {
		err = errors.New("nil chat stream")
		s.log.Error("upstream error", "err", err, "ip", ip)
		http.Error(w, unavailableMsg, http.StatusBadGateway)
		return
	}
	defer func() { _ = stream.Close() }()

	s.writeStream(w, stream)
}

func (s *Server) writeRateLimit(w http.ResponseWriter, ip, reason string, retryAfter time.Duration) {
	s.log.Warn("rate limited", "ip", ip, "reason", reason)
	if retryAfter > 0 {
		seconds := int((retryAfter + time.Second - 1) / time.Second)
		w.Header().Set("Retry-After", strconv.Itoa(seconds))
	}
	message := rateLimitedMsg
	if reason == "per_ip_concurrent" || reason == "global_concurrent" {
		message = busyMsg
	}
	http.Error(w, message, http.StatusTooManyRequests)
}

func isJSON(contentType string) bool {
	mediaType, _, err := mime.ParseMediaType(contentType)
	return err == nil && mediaType == "application/json"
}

// sameOrigin blocks browser requests from other sites. Browsers set Origin on
// cross-origin POSTs; Sec-Fetch-Site covers the cases where Origin is absent.
// Non-browser clients can forge either header, so the rate limiter remains the
// real abuse boundary.
func sameOrigin(r *http.Request) bool {
	if origin := r.Header.Get("Origin"); origin != "" {
		u, err := url.Parse(origin)
		if err != nil || u.Host == "" {
			return false
		}
		return strings.EqualFold(u.Host, r.Host)
	}

	switch r.Header.Get("Sec-Fetch-Site") {
	case "", "same-origin", "none":
		return true
	default:
		return false
	}
}

func isMaxBytesError(err error) bool {
	var maxErr *http.MaxBytesError
	return errors.As(err, &maxErr)
}

// sanitize keeps the last messages, only user/assistant roles, and caps length.
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
		content := cleanContent(m.Content)
		content = truncateUTF8(content, maxContentBytes)
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
			w.Header().Set("Cache-Control", "no-store")
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

// clientIP uses the rightmost X-Forwarded-For entry. When TRUST_PROXY is set,
// the trusted reverse proxy is expected to append the address it observed, so
// the rightmost value is the one a client cannot choose. Falling back to
// RemoteAddr on a malformed header avoids trusting a spoofed leftmost value.
func clientIP(r *http.Request, trustProxy bool) string {
	if trustProxy {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			parts := strings.Split(xff, ",")
			last := strings.TrimSpace(parts[len(parts)-1])
			if ip := net.ParseIP(last); ip != nil {
				return ip.String()
			}
		}
	}
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		if ip := net.ParseIP(host); ip != nil {
			return ip.String()
		}
		return host
	}
	if ip := net.ParseIP(r.RemoteAddr); ip != nil {
		return ip.String()
	}
	return r.RemoteAddr
}
