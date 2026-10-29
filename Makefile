GO ?= go
NPM ?= npm
BINARY ?= avatar
IMAGE ?= avatar-go
TEMPL ?= $(GO) tool templ

.PHONY: all build run generate css assets verify-assets test test-js race e2e eval vet fmt check clean docker

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
	@for f in internal/web/page_templ.go internal/web/assets/app.css internal/web/assets/app.js; do \
		if ! git ls-files --error-unmatch "$$f" >/dev/null 2>&1; then \
			echo "Generated frontend asset is not tracked: $$f"; \
			exit 1; \
		fi; \
	done
	@git diff --exit-code -- internal/web/page_templ.go internal/web/assets/app.css internal/web/assets/app.js || \
		(echo "Generated frontend assets are stale. Run 'make assets' and commit the results."; exit 1)

test:
	$(GO) test ./...

test-js:
	$(NPM) run test:js

e2e:
	$(NPM) run test:e2e

eval:
	$(GO) run ./cmd/eval

race:
	$(GO) test -race ./...

vet:
	$(GO) vet ./...

fmt:
	$(GO) fmt ./...
	$(TEMPL) fmt ./internal/web

check: vet test build

docker:
	docker build -t $(IMAGE) .

clean:
	rm -f $(BINARY)
