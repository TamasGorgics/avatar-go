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
