VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)

.PHONY: build test fmt fmt-check check dev clean plugin plugin-test

build:
	CGO_ENABLED=0 go build -trimpath -ldflags '$(LDFLAGS)' -o bin/kvist ./cmd/kvist

test:
	go vet ./...
	go test -race ./...

fmt:
	gofmt -w cmd internal themes

fmt-check:
	@out=$$(gofmt -l cmd internal themes); if [ -n "$$out" ]; then echo "gofmt needed (run make fmt):"; echo "$$out"; exit 1; fi

# check runs what CI runs.
check: fmt-check test plugin-test
	cd plugin && npm run build

dev:
	go run ./cmd/kvist dev --dir example-vault --config kvist.example.toml

plugin:
	cd plugin && npm ci && npm run build

plugin-test:
	cd plugin && npm ci && npm run typecheck && npm test

clean:
	rm -rf bin plugin/main.js plugin/.test-build
