.PHONY: build test clean release

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT  ?= $(shell git rev-parse HEAD 2>/dev/null || echo "unknown")
DATE    ?= $(shell date -u '+%Y-%m-%d')
LDFLAGS := -X main.Version=$(VERSION) \
           -X main.CommitSHA=$(COMMIT) \
           -X main.BuildDate=$(DATE)

build:
	go build -ldflags "$(LDFLAGS)" -o bin/polvo ./cmd/polvo

test:
	go test ./...

clean:
	rm -rf bin/

release:
	@if [ -z "$(TAG)" ]; then echo "Usage: make release TAG=v0.1.0"; exit 1; fi
	git tag $(TAG)
	git push origin $(TAG)
