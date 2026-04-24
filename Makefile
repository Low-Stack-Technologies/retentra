.PHONY: build clean fmt test test-integration

ifeq ($(origin RETENTRA_GOOGLE_CLIENT_ID), undefined)
ifneq ($(wildcard .env),)
include .env
endif
endif

OUT ?= bin/retentra
GOOGLE_LDFLAGS := -X retentra/internal/retentra.googleClientID=$(RETENTRA_GOOGLE_CLIENT_ID) -X retentra/internal/retentra.googleClientSecret=$(RETENTRA_GOOGLE_CLIENT_SECRET)

build:
	mkdir -p "$(dir $(OUT))"
	go build -ldflags "$(GOOGLE_LDFLAGS)" -o "$(OUT)" ./cmd/retentra

clean:
	rm -rf bin

fmt:
	gofmt -w cmd internal

test:
	go test ./...

test-integration:
	go test -tags integration ./...
