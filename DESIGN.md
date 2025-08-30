# Design Decisions

## Why these choices?

### SQLite over PostgreSQL
**Decision**: Use SQLite as the main database

**Why**: 
- Zero setup - just works out of the box
- Perfect for small to medium workloads
- Easy to backup (single file)
- Can switch to Postgres later if needed

**Downside**: Won't scale to multiple servers easily

### Aggressive URL normalization
**Decision**: Clean up URLs heavily before storing

**Examples**:
- `HTTPS://EXAMPLE.COM/` → `https://example.com`
- `http://site.com:80/` → `http://site.com` 
- `https://site.com?b=2&a=1` → `https://site.com?a=1&b=2`

**Why**: Prevents monitoring the same site multiple times with slightly different URLs

### Worker pools with per-host limits
**Decision**: Max 8 total workers, but only 1 per hostname

**Why**: 
- Don't overwhelm target websites
- Be a good internet citizen
- Still get decent parallelism across different hosts

**How it works**:
```
Global limit: 8 workers max
Per-host limit: 1 worker per hostname
```

### Cursor-based pagination
**Decision**: Use encoded cursor tokens instead of page numbers

**Why**:
- No "page drift" when new items are added
- Better performance on large datasets
- More reliable for concurrent access

**Format**: `base64(timestamp|id)`

### Idempotency with database storage
**Decision**: Store idempotency keys in the database

**Why**: 
- Survives server restarts
- Prevents duplicate registrations reliably
- Simple to implement and understand

## What could be better?

### Current limitations
1. **Single server only** - SQLite doesn't share across machines
2. **No retries yet** - should retry 5xx errors with backoff
3. **Basic error handling** - could be more sophisticated
4. **Memory-based worker state** - doesn't survive restarts

### If this was production
1. **Switch to PostgreSQL** for multi-server deployments
2. **Add Redis** for shared worker coordination
3. **Add metrics** (Prometheus)
4. **Better logging** (structured JSON logs)
5. **Circuit breakers** for failing endpoints
6. **Webhooks** for status change notifications

## Testing strategy

### What we test
- URL canonicalization (comprehensive)
- HTTP API endpoints (using httptest)
- Database operations (in-memory SQLite)
- Idempotency behavior
- Pagination logic

### What we don't test (yet)
- Background worker concurrency
- Retry/backoff behavior  
- Real network failures
- Database migrations in detail

## Performance considerations

### Database indexes
```sql
-- Fast target lookups by host
CREATE INDEX targets_host_created_idx ON targets (host, created_at, id);

-- Fast result queries (newest first)
CREATE INDEX results_target_checked_desc ON check_results (target_id, checked_at DESC);
```

### Memory usage
- Worker pools prevent runaway goroutines
- Database connections pooled automatically
- JSON streaming for large responses

## Security

### Input validation
- URLs validated before storage
- Query parameters bounded (limit, etc.)
- No SQL injection (using parameterized queries)

### What's missing
- Rate limiting per client
- Authentication (not required for assignment)
- HTTPS enforcement (would add in production)

## Deployment

### Development
```bash
go run cmd/main.go
```

### Production
```bash
docker build -t linkwatch .
docker run -p 8080:8080 linkwatch
```

### Environment variables
```bash
CHECK_INTERVAL=30s    # Check frequency
MAX_CONCURRENCY=16    # Worker pool size  
HTTP_TIMEOUT=10s      # Request timeout
```

## Trade-offs made

### Simplicity over features
- Basic retry logic instead of sophisticated circuit breakers
- SQLite instead of distributed database
- Simple worker pools instead of job queues

### Correctness over performance  
- Aggressive URL normalization (slight CPU cost)
- Database transactions for consistency
- Cursor pagination (more complex but more reliable)

### Readability over cleverness
- Clear function names and structure
- Explicit error handling
- Straightforward control flow

## Future improvements

1. **Better monitoring** - add /metrics endpoint
2. **Webhook notifications** - alert when sites go down
3. **Custom check intervals** - per-URL configuration
4. **Geographic checks** - check from multiple locations
5. **Historical trends** - track uptime percentages

---

*Built to be simple, reliable, and easy to understand.*