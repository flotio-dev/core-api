# Makefile for Flotio Core API

SWAG_VERSION ?= v1.16.6

.PHONY: help build run test swagger swagger-install swagger-check clean

# Default target
help:
	@echo "Available commands:"
	@echo "  build           - Build the API binary"
	@echo "  run             - Run the API server"
	@echo "  test            - Run tests"
	@echo "  swagger         - Regenerate Swagger docs into docs/api/ (contract T2)"
	@echo "  swagger-install - Install pinned swag CLI ($(SWAG_VERSION)) with the '@'-in-path fix"
	@echo "  swagger-check   - Regenerate and fail if docs/api/ is out of date (drift check, D3)"
	@echo "  clean           - Clean build artifacts and generated docs"

# Build the API
build:
	go build -o bin/api ./cmd/api/main.go

# Run the API
run:
	go run ./cmd/api/main.go

# Run tests
test:
	go test ./...

# Install pinned swag CLI (never @latest — contract T1). The official v1.16.6
# routerPattern rejects '@' in router paths (/auth/@me), so the one-character
# upstream fix is applied to the pinned source before building (contract T1
# keeps the version pinned; AC-19).
swagger-install:
	go install github.com/swaggo/swag/cmd/swag@$(SWAG_VERSION)
	go run ./tools/swag-patch $(SWAG_VERSION)

# Generate Swagger documentation (contract T2)
swagger: swagger-install
	swag init -g cmd/api/main.go -o docs/api --parseDependency --parseInternal

# Drift check: regenerate and fail on any diff in docs/api (contract §8 / D3)
swagger-check:
	$(MAKE) swagger
	@git diff --exit-code -- docs/api || { echo "Swagger docs are out of date — run 'make swagger' and commit docs/api/"; exit 1; }

# Clean build artifacts and generated docs
clean:
	rm -rf bin/
	rm -rf docs/api/docs.go docs/api/swagger.json docs/api/swagger.yaml
