.PHONY: build test lint check clean

BINARY := agent-sandbox
GO := go

build:
	cd cmd/agent-sandbox && $(GO) build -o ../../$(BINARY) .

test:
	$(GO) test ./...

lint:
	golangci-lint run ./...

check: lint test

clean:
	rm -f $(BINARY)
	rm -rf .build/
