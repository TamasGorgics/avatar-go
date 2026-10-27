package main

import (
	"bufio"
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func envWithout(keys ...string) []string {
	blocked := make(map[string]bool, len(keys))
	for _, key := range keys {
		blocked[key] = true
	}
	env := make([]string, 0, len(os.Environ()))
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if !blocked[key] {
			env = append(env, entry)
		}
	}
	return env
}

func TestServeDrainsInFlightRequests(t *testing.T) {
	startedCh := make(chan struct{})
	releaseCh := make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseCh) }) }
	defer release()

	mux := http.NewServeMux()
	mux.HandleFunc("/slow", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("started"))
		w.(http.Flusher).Flush()
		close(startedCh)
		<-releaseCh
		_, _ = w.Write([]byte("done"))
	})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	srv := &http.Server{Handler: mux}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() { errCh <- serve(ctx, testLogger(), srv, ln, 2*time.Second) }()

	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get("http://" + ln.Addr().String() + "/slow")
	if err != nil {
		t.Fatalf("GET /slow: %v", err)
	}
	defer resp.Body.Close()

	select {
	case <-startedCh:
	case <-time.After(time.Second):
		t.Fatal("handler did not start")
	}

	cancel()

	select {
	case err := <-errCh:
		t.Fatalf("serve returned before handler finished: %v", err)
	case <-time.After(50 * time.Millisecond):
	}

	release()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if got := string(body); got != "starteddone" {
		t.Fatalf("body = %q, want %q", got, "starteddone")
	}

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("serve: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("serve did not return")
	}
}

func TestServeShutdownTimeout(t *testing.T) {
	startedCh := make(chan struct{})
	releaseCh := make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseCh) }) }
	defer release()

	mux := http.NewServeMux()
	mux.HandleFunc("/block", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		close(startedCh)
		<-releaseCh
	})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	srv := &http.Server{Handler: mux}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() { errCh <- serve(ctx, testLogger(), srv, ln, 100*time.Millisecond) }()

	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get("http://" + ln.Addr().String() + "/block")
	if err != nil {
		t.Fatalf("GET /block: %v", err)
	}
	defer resp.Body.Close()

	select {
	case <-startedCh:
	case <-time.After(time.Second):
		t.Fatal("handler did not start")
	}

	cancel()

	select {
	case err := <-errCh:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("serve error = %v, want context.DeadlineExceeded", err)
		}
	case <-time.After(time.Second):
		t.Fatal("serve did not return after shutdown timeout")
	}
}

func TestMainStartsAndShutsDownGracefully(t *testing.T) {
	if os.Getenv("AVATAR_GO_MAIN_TEST") == "1" {
		main()
		return
	}
	if runtime.GOOS == "windows" {
		t.Skip("signal-based shutdown test is not supported on Windows")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	dir := t.TempDir()
	envFile := strings.Join([]string{
		"ADDR=127.0.0.1:0",
		"OPENAI_API_KEY=test-key",
		"LLM_MODEL=dotenv-model",
		"LOG_FORMAT=json",
		"",
	}, "\n")
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(envFile), 0o600); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("executable: %v", err)
	}
	cmd := exec.CommandContext(ctx, exe, "-test.run=^TestMainStartsAndShutsDownGracefully$")
	cmd.Dir = dir
	cmd.Env = append(envWithout(
		"ADDR",
		"OPENAI_API_KEY",
		"LLM_API_KEY",
		"LLM_MODEL",
		"LOG_FORMAT",
		"LOG_LEVEL",
	), "AVATAR_GO_MAIN_TEST=1")
	stderr, err := cmd.StderrPipe()
	if err != nil {
		t.Fatalf("stderr pipe: %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}

	var logs strings.Builder
	scanner := bufio.NewScanner(stderr)
	ready := false
	for scanner.Scan() {
		line := scanner.Text()
		logs.WriteString(line)
		logs.WriteByte('\n')
		if strings.Contains(line, `"msg":"listening"`) {
			ready = true
			break
		}
	}
	if !ready {
		_ = cmd.Wait()
		t.Fatalf("main did not start:\n%s", logs.String())
	}
	if !strings.Contains(logs.String(), `"model":"dotenv-model"`) {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		t.Fatalf(".env was not loaded:\n%s", logs.String())
	}

	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		t.Fatalf("signal: %v", err)
	}

	for scanner.Scan() {
		logs.WriteString(scanner.Text())
		logs.WriteByte('\n')
	}
	if err := scanner.Err(); err != nil {
		t.Errorf("read stderr: %v", err)
	}
	if err := cmd.Wait(); err != nil {
		t.Fatalf("main exited with error: %v\n%s", err, logs.String())
	}
	if !strings.Contains(logs.String(), "shutting down") {
		t.Errorf("main did not log shutdown:\n%s", logs.String())
	}
}

func TestServeReturnsServeError(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	if err := ln.Close(); err != nil {
		t.Fatalf("close listener: %v", err)
	}

	srv := &http.Server{Handler: http.NewServeMux()}
	err = serve(context.Background(), testLogger(), srv, ln, time.Second)
	if err == nil {
		t.Fatal("serve returned nil, want an error")
	}
	if errors.Is(err, http.ErrServerClosed) {
		t.Fatalf("serve returned http.ErrServerClosed, want the listener error")
	}
}
