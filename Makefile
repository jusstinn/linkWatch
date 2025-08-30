.PHONY: build run test test-verbose clean lint docker-build docker-run help

# Build variables
BINARY_NAME=linkwatch
DOCKER_IMAGE=linkwatch
VERSION?=dev
BUILD_TIME=$(shell date -u '+%Y-%m-%d_%H:%M:%S')
GIT_COMMIT=$(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")

# Go variables
GO_VERSION=1.21
LDFLAGS=-ldflags "-X main.Version=${VERSION} -X main.BuildTime=${BUILD_TIME} -X main.GitCommit=${GIT_COMMIT}"

## help: Show this help message
help:
	@echo 'Usage:'
	@sed -n 's/^##//p' ${MAKEFILE_LIST} | column -t -s ':' | sed -e 's/^/ /'

## build: Build the application binary
build:
	@echo "Building ${BINARY_NAME}..."
	CGO_ENABLED=1 go build ${LDFLAGS} -o ${BINARY_NAME} cmd/main.go

## run: Run the application
run:
	@echo "Running ${BINARY_NAME}..."
	go run cmd/main.go

## test: Run all tests
test:
	@echo "Running tests..."
	go test -v -race ./...

## test-verbose: Run tests with verbose output and coverage
test-verbose:
	@echo "Running tests with coverage..."
	go test -v -race -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

## test-coverage: Show test coverage
test-coverage:
	@echo "Calculating test coverage..."
	go test -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out

## lint: Run linter
lint:
	@echo "Running linter..."
	golangci-lint run

## clean: Clean build artifacts
clean:
	@echo "Cleaning..."
	rm -f ${BINARY_NAME}
	rm -f coverage.out coverage.html
	rm -f *.db
	go clean

## deps: Download and verify dependencies
deps:
	@echo "Downloading dependencies..."
	go mod download
	go mod verify

## tidy: Tidy go modules
tidy:
	@echo "Tidying go modules..."
	go mod tidy

## docker-build: Build Docker image
docker-build:
	@echo "Building Docker image..."
	docker build -t ${DOCKER_IMAGE}:${VERSION} .
	docker tag ${DOCKER_IMAGE}:${VERSION} ${DOCKER_IMAGE}:latest

## docker-run: Run application in Docker
docker-run: docker-build
	@echo "Running Docker container..."
	docker run --rm -p 8080:8080 --name ${BINARY_NAME} ${DOCKER_IMAGE}:latest

## docker-test: Test Docker image
docker-test: docker-build
	@echo "Testing Docker image..."
	docker run --rm -d --name ${BINARY_NAME}-test -p 8080:8080 ${DOCKER_IMAGE}:latest
	sleep 5
	curl -f http://localhost:8080/healthz || (docker stop ${BINARY_NAME}-test && exit 1)
	docker stop ${BINARY_NAME}-test
	@echo "Docker image test passed!"

## install: Install the application
install:
	@echo "Installing ${BINARY_NAME}..."
	go install ${LDFLAGS} ./cmd

## dev: Run in development mode with hot reload (requires air)
dev:
	@echo "Starting development server..."
	air -c .air.toml

## migrate: Run database migrations
migrate:
	@echo "Running database migrations..."
	go run cmd/main.go -migrate-only

## security: Run security checks
security:
	@echo "Running security checks..."
	gosec ./...

## fmt: Format code
fmt:
	@echo "Formatting code..."
	go fmt ./...
	goimports -w .

## vet: Run go vet
vet:
	@echo "Running go vet..."
	go vet ./...

## check: Run all checks (fmt, vet, lint, test)
check: fmt vet lint test
	@echo "All checks passed!"

## release: Build release binaries for multiple platforms
release:
	@echo "Building release binaries..."
	mkdir -p dist
	GOOS=linux GOARCH=amd64 CGO_ENABLED=1 go build ${LDFLAGS} -o dist/${BINARY_NAME}-linux-amd64 cmd/main.go
	GOOS=darwin GOARCH=amd64 CGO_ENABLED=1 go build ${LDFLAGS} -o dist/${BINARY_NAME}-darwin-amd64 cmd/main.go
	GOOS=windows GOARCH=amd64 CGO_ENABLED=1 go build ${LDFLAGS} -o dist/${BINARY_NAME}-windows-amd64.exe cmd/main.go
