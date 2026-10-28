# avatar-go

[![CI](https://github.com/TamasGorgics/avatar-go/actions/workflows/ci.yml/badge.svg)](https://github.com/TamasGorgics/avatar-go/actions/workflows/ci.yml)
[![Coverage](https://img.shields.io/endpoint?url=https://raw.githubusercontent.com/TamasGorgics/avatar-go/badges/coverage.json)](https://github.com/TamasGorgics/avatar-go/actions/workflows/ci.yml)

A small Go service that serves a chat page and streams answers from an OpenAI-compatible API.

## Development

Requires Go 1.27.1+ and Node 22.12+ (Node is only needed for frontend asset changes and JavaScript tests).

```sh
cp .env.example .env
# Fill in OPENAI_API_KEY or LLM_API_KEY in .env
make run
```

The server listens on `:8080` by default. See `.env.example` for the other settings.

## Frontend

The page is built with [Templ](https://templ.guide/), [Tailwind CSS](https://tailwindcss.com/),
and TypeScript bundled with esbuild.
Generated files are committed so normal Go builds do not need Node:

- `internal/web/page_templ.go` is generated from `internal/web/page.templ`.
- `internal/web/assets/app.css` is generated from `internal/web/assets/src/app.css`.
- `internal/web/assets/app.js` is generated from `internal/web/assets/src/js/main.ts`.

The favicon and portrait are static assets: `internal/web/assets/favicon.svg` is the TG
monogram, and `internal/web/assets/portrait.jpg` is an optimized 400×400 headshot.

Install frontend dependencies once:

```sh
npm ci
```

After changing the page markup, styles, or behavior:

```sh
make assets
make fmt
```

`make verify-assets` regenerates all three files and fails if the committed output is stale.
`npm run test:js` runs the TypeScript stream and theme tests. `make check` still runs vet,
tests, and a build; use `make race` when touching concurrency.

The page follows the OS light/dark preference on first visit. The header switch stores an
explicit choice in `localStorage`, updates the browser theme color, and is applied before
first paint by a small bootstrap script in the page head.

## Security and limits

`POST /chat` is rate-limited per client IP and globally, with active-stream caps so a slow
request cannot tie up the model indefinitely:

| Variable | Default | Meaning |
| --- | --- | --- |
| `PER_IP_MINUTE` | `3` | Requests per client IP per rolling minute |
| `PER_IP_HOURLY` | `20` | Requests per client IP per rolling hour |
| `GLOBAL_MINUTE` | `20` | Requests across all clients per rolling minute |
| `DAILY_CAP` | `300` | Requests across all clients per calendar day |
| `PER_IP_CONCURRENT` | `1` | Active chat streams per client IP |
| `GLOBAL_CONCURRENT` | `4` | Active chat streams across all clients |

A zero value denies that dimension; invalid or negative values fall back to the default.
IPv6 clients are grouped by `/64` so rotating addresses within one allocation does not
bypass the per-IP limits. State is in-memory and per-process, so limits reset on restart
and are not shared across replicas.

`TRUST_PROXY=1` must only be used behind one trusted reverse proxy that overwrites or
appends `X-Forwarded-For`; the server uses the rightmost entry and falls back to the
connection address if it is not a valid IP. The proxy should also preserve the original
`Host`, because `/chat` checks the browser `Origin` against it.

`/chat` also requires `application/json`, rejects cross-origin browser requests, caps the
body at 16 KB, keeps the last six messages, truncates each message to 500 bytes on a valid
UTF-8 boundary, and removes invisible control characters. Visitor messages are screened
for common prompt-injection phrases before they reach the model. That screening is
best-effort: the durable protections are that the model has no tools, sees only the public
profile, has a hardened system prompt, and is capped at 400 completion tokens. Responses
include a Content Security Policy and the usual framing/MIME/referrer headers; the one
inline theme bootstrap is why the CSP allows `'unsafe-inline'` scripts.

There is no CAPTCHA or WAF in the default deployment. If the public site attracts
distributed bot traffic, put it behind Cloudflare or add Turnstile.

## Docker

The image builds only the Go binary because the generated frontend assets are committed,
so Node is not required:

```sh
make docker
docker run --rm -p 8080:8080 --env-file .env avatar-go
```

Or use Compose:

```sh
docker compose up --build
```

`.env` is excluded from the build context and never baked into the image; configuration is
read from environment variables at runtime. The runtime image is distroless and runs as a
non-root user.
