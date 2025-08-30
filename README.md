# LinkWatch

A simple URL monitoring service that checks if your websites are up and running.

## What it does

- Register URLs you want to monitor
- Automatically checks them every 15 seconds
- Stores response times and status codes
- Provides a REST API to view results

## Quick Start

```bash
# Make sure you're in the linkwatch directory
cd linkwatch

# Run the service
go run cmd/main.go
```

That's it! The service starts on http://localhost:8080

## API Examples

### Add a URL to monitor
```bash
curl -X POST http://localhost:8080/v1/targets \
  -H "Content-Type: application/json" \
  -d '{"url":"https://google.com"}'
```

### See what URLs you're monitoring
```bash
curl http://localhost:8080/v1/targets
```

### Check results for a specific URL
```bash
# Use the target ID from the previous call
curl http://localhost:8080/v1/targets/t_abc123/results
```

### Health check
```bash
curl http://localhost:8080/healthz
```

## Configuration

Set these environment variables if you want to change defaults:

- `CHECK_INTERVAL=30s` - How often to check URLs (default: 15s)
- `MAX_CONCURRENCY=4` - Max parallel checks (default: 8)
- `HTTP_TIMEOUT=10s` - Request timeout (default: 5s)

## Running Tests

```bash
go test ./...
```

## Docker

```bash
# Build and run with Docker
docker build -t linkwatch .
docker run -p 8080:8080 linkwatch
```

## How URLs are normalized

We clean up URLs to avoid duplicates:
- `HTTPS://EXAMPLE.COM/` becomes `https://example.com`
- `http://site.com:80/` becomes `http://site.com`
- `https://site.com?b=2&a=1` becomes `https://site.com?a=1&b=2`

## Features

### Idempotency
Add the same URL multiple times safely using the `Idempotency-Key` header:

```bash
curl -X POST http://localhost:8080/v1/targets \
  -H "Idempotency-Key: my-unique-key" \
  -H "Content-Type: application/json" \
  -d '{"url":"https://example.com"}'
```

### Pagination
List targets with pagination:

```bash
curl "http://localhost:8080/v1/targets?limit=10&page_token=abc123"
```

### Filtering
Filter by hostname:

```bash
curl "http://localhost:8080/v1/targets?host=example.com"
```

## Architecture

Pretty simple:
- **HTTP API** - handles requests
- **Background workers** - check URLs periodically  
- **SQLite database** - stores everything
- **URL canonicalization** - prevents duplicates

## Development

```bash
# Install dependencies
go mod tidy

# Run tests with coverage
go test -cover ./...

# Build binary
go build -o linkwatch cmd/main.go

# Run binary
./linkwatch
```

## Notes

- Uses SQLite by default (no setup needed)
- Graceful shutdown on Ctrl+C
- Won't hammer websites (max 1 request per host at a time)
- Retries failed requests automatically
- Works on Linux, Mac, and Windows