# AGENTS.md

Small Go service (`main` at repo root) that serves a chat page and streams answers from an
OpenAI-compatible API via `github.com/openai/openai-go/v3`. No tests, CI, or Makefile.

## Layout

- `main.go` — wiring only; owns the `//go:embed about.md` profile.
- `internal/config` — env loading (`Load`).
- `internal/ratelimit` — in-memory per-IP/hour and global daily limiter.
- `internal/twin` — system prompt, SDK client, and `Chatter`/`Stream` interfaces.
- `internal/web` — routes, chat handler/sanitization, and page HTML.

`web.Limiter` and `twin.Chatter` exist so the HTTP layer can be tested with fakes; keep
those seams. `ratelimit.NewWithClock` supports time-dependent tests without sleeping.

## Commands

- Run: `go run .` (listens on `:8080` by default)
- Build: `go build -o avatar .`
- Verify: `go vet ./... && go build -o /dev/null .` — no tests yet; exercise `/chat` with
  curl. Plain `go build ./...` leaves an untracked `avatar-go` binary in the repo root.
- Requires Go 1.27.1 per `go.mod`; SDK v3 itself needs Go 1.26+.
- SDK import path is versioned: `github.com/openai/openai-go/v3` (not `.../openai-go`).

## The profile is embedded at compile time

`about.md` is embedded in `main.go` and passed to `twin.New`. `//go:embed` cannot reference
`..`, which is why the profile stays at the repo root while the prompt template lives in
`internal/twin`; if you move the embed, move the file with it. Editing `about.md` does
nothing until the binary is rebuilt/restarted. It is currently the literal text `TODO`, so
the twin knows nothing until it is filled in. The model is instructed to answer only from
this file.

## Configuration

All config is environment variables (read once at startup by `config.Load`):

| Variable | Default | Notes |
| --- | --- | --- |
| `ADDR` | `:8080` | Listen address |
| `OPENAI_API_KEY` / `LLM_API_KEY` | — | Either works; first non-empty wins |
| `LLM_BASE_URL` | `https://api.openai.com/v1` | Must include `/v1`; `/chat/completions` is appended, so no trailing slash |
| `LLM_MODEL` | `gpt-4o-mini` | Any OpenAI-compatible model |
| `DAILY_CAP` | `300` | Global requests per calendar day |
| `PER_IP_HOURLY` | `20` | Requests per client IP per rolling hour |
| `TRUST_PROXY` | unset | Set to `1` only behind a trusted proxy (e.g. Caddy); enables `X-Forwarded-For` |

The client is built with explicit `option.WithAPIKey`/`option.WithBaseURL`, so the SDK's
own defaults do not apply: `OPENAI_BASE_URL` is ignored — use `LLM_BASE_URL`. Missing API
key is not validated at startup; requests fail with `502` and the error is logged. Invalid
integer caps fall back to their defaults.

## SDK notes

- Uses Chat Completions (`client.Chat.Completions.NewStreaming`), not the Responses API, to
  keep arbitrary OpenAI-compatible `LLM_BASE_URL` endpoints working. Do not switch without
  intentionally dropping that compatibility.
- Keep SDK types inside `internal/twin`; `internal/web` should only see `twin.Message`,
  `twin.Chatter`, and `twin.Stream`.
- The SDK handles upstream SSE framing; the HTTP layer only forwards `Stream.Text()`.
- SDK retries default to 2; the 60s context in `internal/web/chat.go` covers retries and
  the full stream.

## Behavior worth preserving

- Routes: `GET /` (embedded HTML template) and `POST /chat` with body
  `{"history":[{"role":"user"|"assistant","content":"..."}]}`. Response is a plain-text
  stream of assistant deltas, not SSE.
- `/chat` sanitizes input deliberately: last 6 messages only, non-user/assistant roles
  dropped (clients cannot inject `system`), each message truncated to 500 bytes, body
  capped at 16 KB. Keep these guards when refactoring.
- Rate limits are checked before JSON parsing, so malformed requests still consume quota.
- Limiter state is in-memory: the daily counter resets on restart and is per-process, so it
  is a spend guard, not a hard quota across replicas/redeploys.
- `max_completion_tokens` is intentional (newer OpenAI models reject `max_tokens`).
- Page HTML/JS is in `internal/web/page.go`; the system prompt is in
  `internal/twin/prompt.go`. There are no template/static files to edit.
