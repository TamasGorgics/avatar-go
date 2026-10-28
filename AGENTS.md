# AGENTS.md

Small Go service (`main` at repo root) that serves a chat page and streams answers from an
OpenAI-compatible API via `github.com/openai/openai-go/v3`. The page is built with Templ
and Tailwind CSS. A small `Makefile` wraps the common build, test, and run commands.
GitHub Actions runs formatting, generated-asset checks, vet, tests, and a build on pushes
to `main` and pull requests; there is no other CI.

## Layout

- `main.go` — wiring, HTTP server lifecycle (graceful shutdown), and the `//go:embed
  about.md` profile.
- `internal/config` — env loading (`Load`) and optional `.env` parsing (`LoadDotEnv`).
- `internal/logging` — `slog` setup: colored text for local terminals, JSON elsewhere.
- `internal/ratelimit` — in-memory per-IP/hour and global daily limiter.
- `internal/twin` — system prompt, SDK client, and `Chatter`/`Stream` interfaces.
- `internal/web` — routes, chat handler/sanitization, access logs, and the Templ page.
  - `page.templ` is the page source; `page_templ.go` is generated and committed.
  - `assets/src/app.css` is the Tailwind source; `assets/src/js/*.ts` is the TypeScript
    source. `assets/app.css` and `assets/app.js` are generated, committed, and embedded
    with the favicon in `assets.go`.
  - `page_templ.go`, `assets/app.css`, and `assets/app.js` must not be edited by hand.
  - The page uses no UI framework because `/chat` returns a plain-text stream; htmx and
    Alpine are intentionally absent. TypeScript is bundled with esbuild.

`web.Limiter` and `twin.Chatter` exist so the HTTP layer can be tested with fakes; keep
those seams. `ratelimit.NewWithClock` supports time-dependent tests without sleeping.

## Commands

- Run: `cp .env.example .env` (fill in the API key), then `make run` (listens on `:8080`
  by default)
- Build: `make build` (writes `./avatar`; override with `make BINARY=other build`)
- Test: `make test`; `make race` when touching concurrency; `make test-js` for the
  TypeScript stream tests.
- Check: `make check` runs vet, tests, and a build.
- Focused test: `go test ./internal/web -run TestChatSuccess`
- Frontend setup: `npm ci` once, then `make assets` after editing `.templ`, CSS, or
  TypeScript files. `make generate` runs Templ only; `make css` runs Tailwind only.
- `make verify-assets` regenerates Templ, Tailwind, and the JavaScript bundle and fails if
  the committed files differ. CI runs this.
- `make fmt` runs `go fmt ./...` and `go tool templ fmt ./internal/web`; `make clean`
  removes the built binary.
- Requires Go 1.27.1 per `go.mod`; SDK v3 itself needs Go 1.26+. Frontend tooling needs
  Node 22.12+ and npm, but normal `go build`/`go test` use the committed generated files.
- SDK import path is versioned: `github.com/openai/openai-go/v3` (not `.../openai-go`).

## Tests

Tests sit beside the code; shared web fakes are in `internal/web/helpers_test.go`.
`main_test.go` covers graceful drain/timeout and re-execs the test binary with
`AVATAR_GO_MAIN_TEST=1` to exercise `main`; that signal test is skipped on Windows.
`internal/twin` tests the SDK against an `httptest` SSE server, so no API key or network is
needed. `internal/logging` tests JSON/text formats, levels, color, and grouped attrs;
`internal/web` checks the access-log fields and severity, page rendering, embedded assets,
and the absence of inline page CSS/JS. `internal/config` tests both environment parsing and
`.env` syntax; the `main` subprocess test proves `.env` is loaded at startup by running
from a temporary directory. `internal/web/assets/src/js/stream.test.ts` uses Vitest and an
injected clock to cover stream pacing, drain, cancellation, and reduced motion; run it
with `make test-js`.

## CI

`.github/workflows/ci.yml` runs on pushes to `main` and pull requests. It uses the Go
version from `go.mod`, installs Node dependencies with `npm ci`, regenerates Templ,
Tailwind, and JavaScript output with `make verify-assets`, runs `npm run test:js`, then
checks formatting, runs `go vet ./...`, `go test ./...` with a coverage profile,
`go test -race ./...`, and `go build -o /dev/null .`.

On pushes to `main`, CI also writes the total coverage to a shields.io endpoint JSON and
publishes it to the dedicated `badges` branch. The README coverage badge reads
`badges/coverage.json`; do not use that branch for anything else.

## The profile is embedded at compile time

`about.md` is embedded in `main.go` and passed to `twin.New`. `//go:embed` cannot reference
`..`, which is why the profile stays at the repo root while the prompt template lives in
`internal/twin`; if you move the embed, move the file with it. Editing `about.md` does
nothing until the binary is rebuilt/restarted. It is currently the literal text `TODO`, so
the twin knows nothing until it is filled in. The model is instructed to answer only from
this file.

## Configuration

All config is environment variables. At startup `main` calls `config.LoadDotEnv(".env")`
first, so a `.env` file in the working directory is applied when present. The file is
optional, `.env` is gitignored, and real environment variables always win over it.
`.env.example` lists every supported name with blank values; copy it to `.env` and fill in
what you need. The loader supports `KEY=VALUE`, `export KEY=VALUE`, `#` comments, and
single/double-quoted values. A malformed `.env` aborts startup with a line-numbered error
rather than running with partial config.

Config is then read once by `config.Load`:

| Variable | Default | Notes |
| --- | --- | --- |
| `ADDR` | `:8080` | Listen address |
| `OPENAI_API_KEY` / `LLM_API_KEY` | — | Either works; first non-empty wins |
| `LLM_BASE_URL` | `https://api.openai.com/v1` | Must include `/v1`; `/chat/completions` is appended, so no trailing slash |
| `LLM_MODEL` | `gpt-4o-mini` | Any OpenAI-compatible model |
| `DAILY_CAP` | `300` | Global requests per calendar day |
| `PER_IP_HOURLY` | `20` | Requests per client IP per rolling hour |
| `TRUST_PROXY` | unset | Set to `1` only behind a trusted proxy (e.g. Caddy); enables `X-Forwarded-For` |
| `LOG_FORMAT` | `auto` | `auto` = colored text on a terminal, JSON otherwise; `text`/`json` force a format |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn`, or `error` |

The client is built with explicit `option.WithAPIKey`/`option.WithBaseURL`, so the SDK's
own defaults do not apply: `OPENAI_BASE_URL` is ignored — use `LLM_BASE_URL`. Missing API
key is not validated at startup; requests fail with `502` and the error is logged. Invalid
integer caps fall back to their defaults; an invalid `LOG_FORMAT` falls back to `auto` and
an invalid `LOG_LEVEL` to `info`.

## Logging

Logs go to stderr through `log/slog`. With `LOG_FORMAT=auto`, stderr being a terminal
selects the development handler: ANSI-colored levels, a shortened source location, and
logfmt-style metadata. Anything else (containers, systemd, pipes, CI) gets
newline-delimited JSON. `NO_COLOR` or `TERM=dumb` disables color without changing formats.

Every HTTP request logs `method`, `path`, `status`, `bytes`, `duration`, and `remote_ip`;
4xx responses log at `WARN` and 5xx at `ERROR`. Rate-limit hits, upstream failures, stream
errors, page-render errors, and shutdown problems include an `err` attribute. API keys are
never logged. `http.Server.ErrorLog` is routed through the same logger.

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

- Routes: `GET /` (Templ page), `GET /assets/` (embedded CSS/JS), and `POST /chat` with
  body `{"history":[{"role":"user"|"assistant","content":"..."}]}`. The chat response is a
  plain-text stream of assistant deltas, not SSE.
- `/chat` sanitizes input deliberately: last 6 messages only, non-user/assistant roles
  dropped (clients cannot inject `system`), each message truncated to 500 bytes, body
  capped at 16 KB. Keep these guards when refactoring.
- Rate limits are checked before JSON parsing, so malformed requests still consume quota.
- Limiter state is in-memory: the daily counter resets on restart and is per-process, so it
  is a spend guard, not a hard quota across replicas/redeploys.
- `main.go` handles SIGINT/SIGTERM with `http.Server.Shutdown` and a 10s drain, so in-flight
  streams can finish. Do not replace it with a bare `http.ListenAndServe`/`log.Fatal`.
- `max_completion_tokens` is intentional (newer OpenAI models reject `max_tokens`).
- Page markup is in `internal/web/page.templ`; page behavior is in
  `internal/web/assets/src/js/`; styles are in `internal/web/assets/src/app.css`. Regenerate
  committed output with `make assets` after changing any source. The system prompt is in
  `internal/twin/prompt.go`.
