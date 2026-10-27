# AGENTS.md

Single-file Go service (`main.go`, package `main`) that serves a chat page and proxies
streaming answers from an OpenAI-compatible API via `github.com/openai/openai-go/v3`.
No tests, CI, or Makefile.

## Commands

- Run: `go run .` (listens on `:8080` by default)
- Build: `go build -o avatar .`
- Verify: `go vet ./... && go build -o /dev/null .` — there is no test suite; exercise `/chat`
  with curl. Plain `go build ./...` drops an untracked `avatar-go` binary in the repo root.
- Requires Go 1.27.1 per `go.mod`; SDK v3 itself needs Go 1.26+.
- SDK import path is versioned: `github.com/openai/openai-go/v3` (not `.../openai-go`).

## The profile is embedded at compile time

`about.md` is pulled in with `//go:embed` and injected into the system prompt. Editing it
does nothing until the binary is rebuilt/restarted. It is currently the literal text
`TODO`, so the twin knows nothing until it is filled in. The model is instructed to answer
only from this file.

## Configuration

All config is environment variables (read once at startup):

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
key is not validated at startup; requests fail with `502` and the upstream error is logged.

## SDK notes

- Uses Chat Completions (`client.Chat.Completions.NewStreaming`), not the Responses API, to
  keep arbitrary OpenAI-compatible `LLM_BASE_URL` endpoints working. Do not switch without
  intentionally dropping that compatibility.
- The SDK handles upstream SSE framing and decoding; `/chat` only forwards `delta.content`.
- SDK retries are left at the default (2); the 60s `context` deadline covers retries and
  the full stream.

## Behavior worth preserving

- Routes: `GET /` (embedded HTML template) and `POST /chat` with body
  `{"history":[{"role":"user"|"assistant","content":"..."}]}`. Response is a plain-text
  stream of assistant deltas, not SSE.
- `/chat` sanitizes input deliberately: last 6 turns only, non-user/assistant roles
  dropped (clients cannot inject `system`), each message truncated to 500 chars, body
  capped at 16 KB. Keep these guards when refactoring.
- Rate limits are checked before JSON parsing, so malformed requests still consume quota.
- Limiter state is in-memory: the daily counter resets on restart and is per-process, so it
  is a spend guard, not a hard quota across replicas/redeploys.
- `max_completion_tokens` is intentional (newer OpenAI models reject `max_tokens`).
- Page HTML/JS and the system prompt are Go string constants in `main.go`; there are no
  template/static files to edit.
