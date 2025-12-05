# Makefile for Flotio Core API

.PHONY: help build run test swagger clean

# Default target
help:
	@echo "Available commands:"
	@echo "  build           - Build the API binary"
	@echo "  run             - Run the API server"
	@echo "  test            - Run tests"
	@echo "  swagger         - Generate Swagger documentation"
	@echo "  swagger-install - Install swag CLI tool"
	@echo "  clean           - Clean build artifacts"

# Build the API
build:
	go build -o bin/api ./cmd/main.go

# Run the API
run:
	go run ./cmd/main.go

# Run tests
test:
	go test ./...

# Install swag CLI if not present
swagger-install:
	@which swag > /dev/null || go install github.com/swaggo/swag/cmd/swag@latest

# Generate Swagger documentation
swagger: swagger-install
	swag init -g cmd/main.go -o docs --parseDependency --parseInternal

# Clean build artifacts
clean:
	rm -rf bin/
	rm -rf docs/docs.go docs/swagger.json docs/swagger.yaml
