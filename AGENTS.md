# AGENTS.md

Small Go service (`main` at repo root) that serves a chat page and streams answers from an
OpenAI-compatible API via `github.com/openai/openai-go/v3`. The page is built with Templ
and Tailwind CSS. A small `Makefile` wraps the common build, test, and run commands.
GitHub Actions runs formatting, generated-asset checks, vet, Go and JavaScript tests, a
build, and browser E2E on pushes to `main` and pull requests. A separate manual prompt-eval
workflow makes real model calls and is not part of normal CI.

## Layout

- `main.go` — wiring, HTTP server lifecycle (graceful shutdown), and the `//go:embed
  about.md` profile.
- `cmd/eval` — prompt-eval CLI. It loads the same config and profile as the server, skips
  profile-dependent cases while `about.md` is `TODO`, and exits non-zero on a failed case.
- `internal/eval` — eval corpus loading/validation, profile readiness, answer grading, and
  stream collection. It has no model or SDK dependency.
- `internal/config` — env loading (`Load`) and optional `.env` parsing (`LoadDotEnv`).
- `internal/logging` — `slog` setup: colored text for local terminals, JSON elsewhere.
- `internal/ratelimit` — in-memory minute/hour/day request limits plus active-stream
  caps, with IPv6 `/64` grouping and idle-state pruning.
- `internal/twin` — system prompt, SDK client, and `Chatter`/`Stream` interfaces.
- `internal/web` — routes, chat handler/sanitization, prompt-injection screening,
  security headers, access logs, and the Templ page.
  - `page.templ` is the page source; `page_templ.go` is generated and committed.
  - `assets/src/app.css` is the Tailwind source; `assets/src/js/*.ts` is the TypeScript
    source. `assets/app.css` and `assets/app.js` are generated, committed, and embedded
    with the favicon and portrait in `assets.go`. `assets/portrait.jpg` is an optimized
    400×400 headshot; `assets/favicon.svg` is the TG monogram.
  - `page_templ.go`, `assets/app.css`, and `assets/app.js` must not be edited by hand.
  - The page uses no UI framework because `/chat` returns a plain-text stream; htmx and
    Alpine are intentionally absent. TypeScript is bundled with esbuild. The light/dark
    switch lives in `assets/src/js/theme.ts`; colors are semantic tokens in
    `assets/src/app.css`.
  - `assets/src/js/chat.ts` is the turn-based chat controller. Completed and stopped turns
    are the source of truth; failed attempts never enter model history. `chat-history.ts`,
    `chat-storage.ts`, and `failure.ts` hold the pure history, session, and error logic so
    they can be tested without a DOM.
  - `testdata/screening.json` is the no-model injection-screen corpus. It contains attacks
    and benign questions that must not be rejected, so rules can be tuned without guessing.
- `e2e/` — Playwright specs plus a deterministic fake OpenAI-compatible server and app
  launcher. `playwright.config.ts` starts both; `tsconfig.e2e.json` type-checks the specs.
- `Dockerfile`, `.dockerignore`, and `compose.yaml` — multi-stage image and local Compose
  service. The image build only needs Go because generated frontend assets are committed.

`web.Limiter` and `twin.Chatter` exist so the HTTP layer can be tested with fakes; keep
those seams. `ratelimit.NewWithClock` supports time-dependent tests without sleeping.

## Commands

- Run: `cp .env.example .env` (fill in the API key), then `make run` (listens on `:8080`
  by default)
- Build: `make build` (writes `./avatar`; override with `make BINARY=other build`)
- Test: `make test`; `make race` when touching concurrency; `make test-js` for the
  TypeScript stream and theme tests.
- Browser tests: `npm ci` and `npx playwright install chromium` once, then `make e2e`;
  `npm run check:e2e` type-checks the specs. E2E uses a local fake upstream and needs no
  API key or network.
- Prompt evals: `make eval` or `go run ./cmd/eval` makes real model calls. Use
  `go run ./cmd/eval -dry-run` to validate the corpus without calling the model,
  `-strict-profile` to fail while `about.md` is still `TODO`, and `-v` to print answers.
- Check: `make check` runs vet, tests, and a build; it does not run E2E or model evals.
- Focused test: `go test ./internal/web -run TestChatSuccess`
- Frontend setup: `npm ci` once, then `make assets` after editing `.templ`, CSS, or
  TypeScript files. `make generate` runs Templ only; `make css` runs Tailwind only.
- `make verify-assets` regenerates Templ, Tailwind, and the JavaScript bundle and fails if
  the committed files differ. CI runs this.
- `make fmt` runs `go fmt ./...` and `go tool templ fmt ./internal/web`; `make clean`
  removes the built binary.
- Docker: `make docker` builds the `avatar-go` image (override with `IMAGE=...`); run it
  with `--env-file .env`, or use `docker compose up --build`.
- Requires Go 1.27.1 per `go.mod`; SDK v3 itself needs Go 1.26+. Frontend tooling needs
  Node 22.12+ and npm, but normal `go build`/`go test` use the committed generated files.
- SDK import path is versioned: `github.com/openai/openai-go/v3` (not `.../openai-go`).

## Tests

Tests sit beside the code; shared web fakes are in `internal/web/helpers_test.go`.
`main_test.go` covers graceful drain/timeout and re-execs the test binary with
`AVATAR_GO_MAIN_TEST=1` to exercise `main`; that signal test is skipped on Windows.
`internal/twin` tests the SDK against an `httptest` SSE server, including optional usage
chunks and fallback for endpoints that reject `stream_options`, so no API key or network is
needed. `internal/logging` tests JSON/text formats, levels, color, and grouped attrs;
`internal/web` checks the access-log fields and severity, page rendering, embedded assets,
security headers, the single theme bootstrap inline script, and the absence of inline CSS.
Its chat tests cover JSON/origin enforcement, body overflow, UTF-8-safe truncation,
injection rejection, X-Forwarded-For handling, that invalid requests do not reserve model
budgets, limiter release, request IDs, health/readiness, and metrics. `internal/ratelimit`
tests attempt versus model-call windows, daily token reservation and settlement, concurrency
caps, idempotent release, IPv6 `/64` keys, and idle pruning against an injected clock.
`internal/config` tests both environment
parsing and `.env` syntax; the `main` subprocess test proves `.env` is loaded at startup by
running from a temporary directory. `internal/eval` tests corpus validation, profile gating,
whole-word grading, and stream collection with fakes. `internal/web/screening_test.go` runs
both attack and benign cases from `testdata/screening.json`, so screening changes do not
silently reject legitimate questions. `e2e/` exercises the real page in Chromium against a
deterministic fake upstream: successful streaming and reload restore, `429` countdown, `400`
editing, `502` retry, Stop, theme persistence, and mobile layout.
`internal/web/assets/src/js/stream.test.ts` uses Vitest and an injected clock to cover
stream pacing, drain, cancellation, and reduced motion; `theme.test.ts` covers stored and
system theme resolution; `chat-history.test.ts` covers last-six request history and
stopped/empty answers; `chat-storage.test.ts` covers the versioned session schema, size
caps, and corrupt data; `failure.test.ts` covers `Retry-After` parsing, failure
classification, and countdown formatting. Run all with `make test-js`. The controller's DOM
behavior is covered by the Playwright E2E specs; keep the Vitest layer DOM-free unless that
decision is revisited.

## CI

`.github/workflows/ci.yml` runs on pushes to `main` and pull requests. It uses the Go
version from `go.mod`, installs Node dependencies with `npm ci`, regenerates Templ,
Tailwind, and JavaScript output with `make verify-assets`, runs `npm run test:js`, then
checks formatting, runs `go vet ./...`, `go test ./...` with a coverage profile,
`go test -race ./...`, `go build -o /dev/null .`, and a Docker image build. A separate
`e2e` job type-checks the Playwright specs, installs Chromium, and runs the browser suite
against the fake upstream.

On pushes to `main`, CI also writes the total coverage to a shields.io endpoint JSON and
publishes it to the dedicated `badges` branch. The README coverage badge reads
`badges/coverage.json`; do not use that branch for anything else.

Prompt evals run only from `.github/workflows/prompt-eval.yml`, manually or on an opt-in
nightly schedule. The workflow uses `-strict-profile`, so it fails until `about.md` is
populated rather than spending tokens on placeholder content. Keep real model calls out of
`ci.yml`.

## The profile is embedded at compile time

`about.md` is embedded in `main.go` and passed to `twin.New`. `//go:embed` cannot reference
`..`, which is why the profile stays at the repo root while the prompt template lives in
`internal/twin`; if you move the embed, move the file with it. Editing `about.md` does
nothing until the binary is rebuilt/restarted. It is currently the literal text `TODO`, so
the twin knows nothing until it is filled in. The model is instructed to answer only from
this file. The eval CLI reads the same file directly; profile-dependent cases stay skipped
until it is ready, and `-strict-profile` makes that state an error.

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
| `PER_IP_MINUTE` | `3` | Requests per client IP per rolling minute |
| `PER_IP_HOURLY` | `20` | Requests per client IP per rolling hour |
| `PER_IP_DAILY_TOKENS` | `20000` | Estimated model tokens per client IP per calendar day |
| `GLOBAL_MINUTE` | `20` | Validated model calls admitted across all clients per rolling minute |
| `DAILY_CAP` | `300` | Validated model calls admitted across all clients per calendar day |
| `PER_IP_CONCURRENT` | `1` | Active chat streams per client IP |
| `GLOBAL_CONCURRENT` | `4` | Active chat streams across all clients |
| `TRUST_PROXY` | unset | Set to `1` only behind one trusted proxy (e.g. Caddy); enables rightmost `X-Forwarded-For` |
| `LOG_FORMAT` | `auto` | `auto` = colored text on a terminal, JSON otherwise; `text`/`json` force a format |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn`, or `error` |

The client is built with explicit `option.WithAPIKey`/`option.WithBaseURL`, so the SDK's
own defaults do not apply: `OPENAI_BASE_URL` is ignored — use `LLM_BASE_URL`. Missing API
key is not validated at startup; requests fail with `502` and the error is logged. A zero
cap denies that dimension; invalid or negative integer caps fall back to their defaults.
An invalid `LOG_FORMAT` falls back to `auto` and an invalid `LOG_LEVEL` to `info`.

## Logging

Logs go to stderr through `log/slog`. With `LOG_FORMAT=auto`, stderr being a terminal
selects the development handler: ANSI-colored levels, a shortened source location, and
logfmt-style metadata. Anything else (containers, systemd, pipes, CI) gets
newline-delimited JSON. `NO_COLOR` or `TERM=dumb` disables color without changing formats.

Every HTTP request logs `method`, `path`, `status`, `bytes`, `duration`, `remote_ip`, and
`request_id`; 4xx responses log at `WARN` and 5xx at `ERROR`. The server-generated
`X-Request-ID` response header correlates errors with request-scoped logs. Rate-limit hits
log `ip` and `reason`;
injection rejections log `ip` and the rule reason, never the message content. Upstream
failures, stream errors, page-render errors, and shutdown problems include an `err`
attribute. API keys are never logged. `http.Server.ErrorLog` is routed through the same
logger.

`GET /healthz` is liveness only; `GET /readyz` reflects local readiness and graceful
shutdown state without probing the model. `GET /metrics` serves aggregate Prometheus
metrics. Keep labels bounded to normalized routes, methods, statuses, limiter reasons, and
token-usage source; never label with IPs, request IDs, or message content. Restrict metrics
scraping at the proxy when needed. The distroless runtime relies on an orchestrator HTTP
probe rather than an in-container shell.

## SDK notes

- Uses Chat Completions (`client.Chat.Completions.NewStreaming`), not the Responses API, to
  keep arbitrary OpenAI-compatible `LLM_BASE_URL` endpoints working. Do not switch without
  intentionally dropping that compatibility.
- Keep SDK types inside `internal/twin`; `internal/web` should only see `twin.Message`,
  `twin.Chatter`, and `twin.Stream`.
- The SDK handles upstream SSE framing; the HTTP layer forwards only `Stream.Text()` to
  clients and reads optional `Stream.TokenUsage()` for budget settlement.
- SDK retries default to 2; the 60s context in `internal/web/chat.go` covers retries and
  the full stream.
- `twin.Stream` exposes optional prompt, completion, and total-token usage without leaking
  SDK types. The client requests usage metadata when supported, retries without that option
  on an explicit unsupported-parameter response, and remembers that endpoint capability for
  the process.
- Per-IP token admission reserves an estimate based on the system prompt, sanitized history,
  chat framing, and the 400-token completion maximum. Reported stream usage settles that
  reservation; if usage is absent or the stream is interrupted, the estimate remains charged.
  This is an approximate, in-memory spend guard, not an exact cross-replica quota.

## Behavior worth preserving

- Routes: `GET /` (Templ page), `GET /assets/` (embedded CSS/JS), and `POST /chat` with
  body `{"history":[{"role":"user"|"assistant","content":"..."}]}`. The chat response is a
  plain-text stream of assistant deltas, not SSE.
- `/chat` sanitizes input deliberately: it keeps the last 6 submitted messages, drops
  non-user/assistant roles (clients cannot inject `system`), cleans invisible
  control/format characters, truncates each message to 500 bytes on a UTF-8 boundary,
  caps the body at 16 KB with `413` on overflow, and requires the final message to be a
  non-empty user message. Keep these guards when refactoring.
- `/chat` requires `application/json` and rejects browser requests whose `Origin` (or
  `Sec-Fetch-Site`) is cross-site. Non-browser clients can forge both headers, so these
  checks are CSRF hardening, not authentication.
- Visitor messages are screened for high-confidence prompt-injection phrases in
  `internal/web/guard.go`, including spaced, fullwidth, and zero-width variants and
  phrases split across messages. This is best-effort; the real limits are no tools,
  public-profile-only context, a hardened prompt, and a 400-token completion cap.
  Assistant messages are not screened because they are model output.
- Per-IP minute/hour attempt limits are checked before JSON parsing, so malformed requests
  still consume that anti-abuse quota. Global-minute/daily model-call limits, per-IP token
  reservations, and active-stream slots are checked only after validation, immediately before
  the upstream call. Limiter state is in-memory and per-process, so it is not a hard quota
  across replicas/redeploys.
- Time-window `429`s include `Retry-After` in whole seconds, rounded up from the limiter's
  estimate. Concurrency `429`s omit it and use a distinct busy message, because the server
  cannot predict when a stream slot will free up. Keep that distinction when changing the
  limiter.
- The client treats completed and stopped turns as its transcript and model history. A
  failed attempt never enters history: `Retry` reuses the question and user bubble,
  `Edit` restores the question to the input, and `Stop` commits a partial answer only when
  text arrived. Completed and stopped turns are restored from `sessionStorage` under
  `avatar-go:chat:v1` (capped at 50 turns/32 KB); failed attempts and countdowns are not
  persisted. Clear aborts an in-flight answer and removes the stored transcript.
- The first six messages sent to `/chat` come from `turnsToHistory`, with the pending
  question last. Keep the last-six rule when adding conversation controls.
- Browser tests must keep using the deterministic fake upstream in `e2e/support/`; only the
  prompt-eval CLI may make real model calls. The fake scenarios are keyed by the `e2e:*`
  question prefix, so keep those questions out of the eval corpus.
- The eval CLI calls `twin.Chatter` directly; it does not exercise the HTTP screen or
  limiter. Injection cases there test prompt resilience, while
  `internal/web/screening_test.go` covers the screen itself.
- Prompt-eval factual cases stay marked `requires_profile` until `about.md` is populated.
  Add specific facts to `internal/eval/testdata/cases.json` as the profile grows.
- Security headers (CSP, `X-Frame-Options: DENY`, `nosniff`, referrer and permissions
  policies) are applied to every response. The CSP allows `'unsafe-inline'` scripts only
  because of the single inline theme bootstrap; adding scripts should otherwise use
  external files. Chat responses are `no-store`.
- `main.go` sets read, write, idle, and header limits on `http.Server` (the 75s write
  timeout intentionally exceeds the 60s chat context) and handles SIGINT/SIGTERM with
  `http.Server.Shutdown` and a 10s drain, so in-flight streams can finish. Do not replace
  it with a bare `http.ListenAndServe`/`log.Fatal`.
- `max_completion_tokens` is intentional (newer OpenAI models reject `max_tokens`).
- The page follows the OS light/dark preference until the header switch stores an explicit
  `light` or `dark` choice in `localStorage`. The tiny inline bootstrap in `page.templ` is
  the only inline script and must stay before the stylesheet so the theme is applied
  without a flash. Colors are semantic tokens in `app.css`, not `dark:` utilities.
- Page markup is in `internal/web/page.templ`; page behavior is in
  `internal/web/assets/src/js/`; styles are in `internal/web/assets/src/app.css`. Regenerate
  committed output with `make assets` after changing any source. The system prompt is in
  `internal/twin/prompt.go`.
