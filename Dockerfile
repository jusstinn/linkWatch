# Build stage
FROM golang:1.21-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git ca-certificates tzdata

# Set working directory
WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=1 GOOS=linux go build -a -installsuffix cgo -o linkwatch cmd/main.go

# Final stage
FROM alpine:latest

# Install runtime dependencies
RUN apk --no-cache add ca-certificates tzdata wget

# Create non-root user
RUN addgroup -g 1001 -S linkwatch && \
    adduser -u 1001 -S linkwatch -G linkwatch

# Set working directory
WORKDIR /app

# Copy binary from builder stage
COPY --from=builder /app/linkwatch .

# Copy migrations
COPY --from=builder /app/migrations ./migrations

# Change ownership to non-root user
RUN chown -R linkwatch:linkwatch /app

# Switch to non-root user
USER linkwatch

# Expose port
EXPOSE 8080

# Set default environment variables
ENV CHECK_INTERVAL=15s \
    MAX_CONCURRENCY=8 \
    HTTP_TIMEOUT=5s \
    SHUTDOWN_GRACE=10s \
    DATABASE_URL="file:linkwatch.db?_pragma=busy_timeout(5000)"

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/healthz || exit 1

# Run the application
CMD ["./linkwatch"]
