.PHONY: build clean fmt test test-integration

build:
	go build -o bin/retentra ./cmd/retentra

clean:
	rm -rf bin

fmt:
	gofmt -w cmd internal

test:
	go test ./...

test-integration:
	go test -tags integration ./...
