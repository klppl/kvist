VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)

.PHONY: build test fmt clean

build:
	CGO_ENABLED=0 go build -trimpath -ldflags '$(LDFLAGS)' -o bin/kvist ./cmd/kvist

test:
	go vet ./...
	go test -race ./...

fmt:
	gofmt -w cmd internal

clean:
	rm -rf bin
