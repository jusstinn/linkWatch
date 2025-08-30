# Simple Dockerfile for LinkWatch
FROM golang:1.21-alpine AS builder

WORKDIR /app

# Copy everything and build
COPY . .
RUN go mod tidy
RUN CGO_ENABLED=1 go build -o linkwatch cmd/main.go

# Final stage
FROM alpine:latest

RUN apk --no-cache add ca-certificates

WORKDIR /app

# Copy binary and migrations
COPY --from=builder /app/linkwatch .
COPY --from=builder /app/migrations ./migrations

EXPOSE 8080

CMD ["./linkwatch"]