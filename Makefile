GO ?= go
GOLANGCI_LINT ?= golangci-lint

.PHONY: vet test lint ci

vet:
	$(GO) vet ./...

test:
	$(GO) test -race -cover ./...

lint:
	$(GOLANGCI_LINT) run

ci: vet lint test
