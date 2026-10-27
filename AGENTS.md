# AGENTS.md

Single-file Go service (`main.go`, package `main`) that serves a chat page and proxies
streaming answers from an OpenAI-compatible API. No dependencies, tests, CI, or Makefile.

## Commands

- Run: `go run .` (listens on `:8080` by default)
- Build: `go build -o avatar .`
- Verify: `go vet ./... && go build ./...` — there is no test suite; exercise `/chat` with curl.
- Requires Go 1.27.1 per `go.mod` (method-pattern routing like `GET /{$}` needs Go 1.22+).

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

Missing API key is not validated at startup; requests fail with `502` and the upstream
error is logged.

## Behavior worth preserving

- Routes: `GET /` (embedded HTML template) and `POST /chat` with body
  `{"history":[{"role":"user"|"assistant","content":"..."}]}`. Response is a plain-text
  stream of assistant deltas, not SSE (upstream SSE is parsed and stripped).
- `/chat` sanitizes input deliberately: last 6 turns only, non-user/assistant roles
  dropped (clients cannot inject `system`), each message truncated to 500 chars, body
  capped at 16 KB. Keep these guards when refactoring.
- Rate limits are checked before JSON parsing, so malformed requests still consume quota.
- Limiter state is in-memory: the daily counter resets on restart and is per-process, so it
  is a spend guard, not a hard quota across replicas/redeploys.
- `max_completion_tokens` is intentional (newer OpenAI models reject `max_tokens`).
- Page HTML/JS and the system prompt are Go string constants in `main.go`; there are no
  template/static files to edit.
