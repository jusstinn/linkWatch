-- Initial schema for LinkWatch

CREATE TABLE targets (
  id TEXT PRIMARY KEY,
  url TEXT NOT NULL UNIQUE,
  host TEXT NOT NULL,
  created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX targets_host_created_idx 
  ON targets (host, created_at, id);

CREATE TABLE check_results (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  target_id TEXT NOT NULL REFERENCES targets(id) ON DELETE CASCADE,
  checked_at TEXT NOT NULL,
  status_code INTEGER NULL,
  latency_ms INTEGER NOT NULL,
  error TEXT NULL
);

CREATE INDEX results_target_checked_desc 
  ON check_results (target_id, checked_at DESC);

CREATE TABLE idempotency_keys (
  key TEXT PRIMARY KEY,
  request_hash TEXT NOT NULL,
  target_id TEXT NOT NULL REFERENCES targets(id),
  response_code INTEGER NOT NULL,
  response_body TEXT NOT NULL, -- store JSON as plain text
  created_at TEXT NOT NULL DEFAULT (datetime('now'))
);
