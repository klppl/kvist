VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)

.PHONY: build test fmt clean plugin plugin-test

build:
	CGO_ENABLED=0 go build -trimpath -ldflags '$(LDFLAGS)' -o bin/kvist ./cmd/kvist

test:
	go vet ./...
	go test -race ./...

fmt:
	gofmt -w cmd internal

plugin:
	cd plugin && npm ci && npm run build

plugin-test:
	cd plugin && npm ci && npm run typecheck && npm test

clean:
	rm -rf bin plugin/main.js plugin/.test-build
