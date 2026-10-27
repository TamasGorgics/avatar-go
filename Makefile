GO ?= go
BINARY ?= avatar

.PHONY: all build run test race vet fmt check clean

all: build

build:
	$(GO) build -o $(BINARY) .

run:
	$(GO) run .

test:
	$(GO) test ./...

race:
	$(GO) test -race ./...

vet:
	$(GO) vet ./...

fmt:
	$(GO) fmt ./...

check: vet test build

clean:
	rm -f $(BINARY)
