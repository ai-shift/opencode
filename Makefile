.PHONY: test vet fmt example
-include .env

test: vet
	go test ./... $(ARGS)

vet: fmt
	go vet ./...
	staticcheck ./...

fmt:
	go fmt ./...

DIR ?=

example:
	@if [ -n "$(DIR)" ]; then \
		go run cmd/example/main.go -dir "$(DIR)"; \
	else \
		go run cmd/example/main.go; \
	fi
