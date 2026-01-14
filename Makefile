.PHONY: test vet fmt example
-include .env

test: vet
	go test ./... $(ARGS)

vet: fmt
	go vet ./...
	staticcheck ./...

fmt:
	go fmt ./...

QUERY ?= what is 2+2
DIR ?=

example:
	@if [ -n "$(DIR)" ]; then \
		go run cmd/example/main.go -query "$(QUERY)" -dir "$(DIR)"; \
	else \
		go run cmd/example/main.go -query "$(QUERY)"; \
	fi
