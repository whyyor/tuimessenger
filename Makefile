# goolm: pure-Go olm (no libolm C library); sqlite_fts5: full-text search in the client database.
TAGS := goolm,sqlite_fts5

.PHONY: build test vet

# VERSION is set for releases, e.g. make build VERSION=v0.1.0; otherwise the binary describes its commit.
VERSION ?=

build:
	go build -tags $(TAGS) -ldflags "-X main.version=$(VERSION)" -o tuimessenger ./cmd/tuimessenger

test:
	go test -tags $(TAGS) ./...

vet:
	go vet -tags $(TAGS) ./...
	go vet -tags $(TAGS),devdummy ./...
