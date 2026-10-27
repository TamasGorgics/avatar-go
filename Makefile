GO ?= go
NPM ?= npm
BINARY ?= avatar
TEMPL ?= $(GO) tool templ

.PHONY: all build run generate css assets verify-assets test race vet fmt check clean

all: build

build:
	$(GO) build -o $(BINARY) .

run:
	$(GO) run .

generate:
	$(TEMPL) generate

css:
	$(NPM) run css

assets: generate
	$(NPM) run assets

verify-assets: assets
	@for f in internal/web/page_templ.go internal/web/assets/app.css; do \
		if ! git ls-files --error-unmatch "$$f" >/dev/null 2>&1; then \
			echo "Generated frontend asset is not tracked: $$f"; \
			exit 1; \
		fi; \
	done
	@git diff --exit-code -- internal/web/page_templ.go internal/web/assets/app.css || \
		(echo "Generated frontend assets are stale. Run 'make assets' and commit the results."; exit 1)

test:
	$(GO) test ./...

race:
	$(GO) test -race ./...

vet:
	$(GO) vet ./...

fmt:
	$(GO) fmt ./...
	$(TEMPL) fmt ./internal/web

check: vet test build

clean:
	rm -f $(BINARY)
