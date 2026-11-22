BINARY := ytd
VERSION := $(shell git describe --tags --abbrev=0 2>/dev/null || echo x.x.x)
SOURCE := $(shell git describe --tags 2>/dev/null || echo local)
LDFLAGS := -X main.version=$(VERSION) -X main.buildSource=$(SOURCE)

.PHONY: all build build-local test fmt clean

all: build-local

build:
	go build -ldflags "$(LDFLAGS)" -o bin/$(BINARY) .

build-local:
	go build -o bin/$(BINARY) .

test:
	go test ./...

fmt:
	gofmt -w *.go

clean:
	rm -rf bin
