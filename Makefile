# goolm: pure-Go olm (no libolm C library); sqlite_fts5: full-text search in the client database.
TAGS := goolm,sqlite_fts5

.PHONY: build test vet

build:
	go build -tags $(TAGS) -o tuimessenger ./cmd/tuimessenger

test:
	go test -tags $(TAGS) ./...

vet:
	go vet -tags $(TAGS) ./...
	go vet -tags $(TAGS),devdummy ./...
