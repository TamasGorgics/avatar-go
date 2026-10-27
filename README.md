# avatar-go

[![CI](https://github.com/TamasGorgics/avatar-go/actions/workflows/ci.yml/badge.svg)](https://github.com/TamasGorgics/avatar-go/actions/workflows/ci.yml)
[![Coverage](https://img.shields.io/endpoint?url=https://raw.githubusercontent.com/TamasGorgics/avatar-go/badges/coverage.json)](https://github.com/TamasGorgics/avatar-go/actions/workflows/ci.yml)

A small Go service that serves a chat page and streams answers from an OpenAI-compatible API.

## Development

Requires Go 1.27.1+ and Node 22+ (Node is only needed for frontend asset changes).

```sh
cp .env.example .env
# Fill in OPENAI_API_KEY or LLM_API_KEY in .env
make run
```

The server listens on `:8080` by default. See `.env.example` for the other settings.

## Frontend

The page is built with [Templ](https://templ.guide/) and [Tailwind CSS](https://tailwindcss.com/).
Generated files are committed so normal Go builds do not need Node:

- `internal/web/page_templ.go` is generated from `internal/web/page.templ`.
- `internal/web/assets/app.css` is generated from `internal/web/assets/src/app.css`.

Install frontend dependencies once:

```sh
npm ci
```

After changing the page markup or styles:

```sh
make assets
make fmt
```

`make verify-assets` regenerates both files and fails if the committed output is stale.
`make check` still runs vet, tests, and a build; use `make race` when touching concurrency.
