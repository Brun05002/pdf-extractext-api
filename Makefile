GO ?= go
GOLANGCI_LINT ?= golangci-lint

.PHONY: vet test lint ci postman

vet:
	$(GO) vet ./...

test:
	$(GO) test -race -cover ./...

lint:
	$(GOLANGCI_LINT) run

ci: vet lint test

# Ejecuta la suite de pruebas de Postman con Newman.
postman:
	@cd docs/postman && npx --yes newman run pdf-extractext.postman_collection.json \
		-e pdf-extractext-local.postman_environment.json \
		--insecure \
		--reporters cli
