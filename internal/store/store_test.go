package store

import (
	"context"
	"database/sql"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func setupTestDB(t *testing.T) *SQLiteStore {
	// Create temporary database
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}

	// Run migrations
	if err := RunMigrations(db, "../../migrations"); err != nil {
		t.Fatalf("Failed to run migrations: %v", err)
	}

	return NewSQLiteStore(db)
}

func TestCursorPagination(t *testing.T) {
	store := setupTestDB(t)
	ctx := context.Background()

	// Create test targets with different timestamps
	targets := []struct {
		url  string
		host string
	}{
		{"https://example1.com", "example1.com"},
		{"https://example2.com", "example2.com"},
		{"https://example3.com", "example3.com"},
	}

	var createdTargets []*Target
	for _, target := range targets {
		created, _, err := store.UpsertTargetByURL(ctx, target.url, target.host)
		if err != nil {
			t.Fatalf("Failed to create target: %v", err)
		}
		createdTargets = append(createdTargets, created)
		// Add small delay to ensure different timestamps
		time.Sleep(time.Millisecond)
	}

	// Test pagination with limit
	firstPage, cursor, err := store.GetTargets(ctx, "", time.Time{}, "", 2)
	if err != nil {
		t.Fatalf("Failed to get first page: %v", err)
	}

	if len(firstPage) != 2 {
		t.Errorf("Expected 2 targets in first page, got %d", len(firstPage))
	}

	if cursor == nil {
		t.Error("Expected cursor for next page")
	}

	// Get second page using cursor
	secondPage, _, err := store.GetTargets(ctx, "", cursor.CreatedAt, cursor.ID, 2)
	if err != nil {
		t.Fatalf("Failed to get second page: %v", err)
	}

	if len(secondPage) != 1 {
		t.Errorf("Expected 1 target in second page, got %d", len(secondPage))
	}

	// Verify we got all targets across pages
	totalTargets := len(firstPage) + len(secondPage)
	if totalTargets != 3 {
		t.Errorf("Expected 3 total targets across pages, got %d", totalTargets)
	}

	// Verify no duplicates between pages
	for _, t1 := range firstPage {
		for _, t2 := range secondPage {
			if t1.ID == t2.ID {
				t.Error("Found duplicate target between pages")
			}
		}
	}
}

func TestHostFiltering(t *testing.T) {
	store := setupTestDB(t)
	ctx := context.Background()

	// Create targets with different hosts
	targets := []struct {
		url  string
		host string
	}{
		{"https://example.com", "example.com"},
		{"https://google.com", "google.com"},
		{"https://example.com/path", "example.com"},
	}

	for _, target := range targets {
		_, _, err := store.UpsertTargetByURL(ctx, target.url, target.host)
		if err != nil {
			t.Fatalf("Failed to create target: %v", err)
		}
	}

	// Filter by host
	filtered, _, err := store.GetTargets(ctx, "example.com", time.Time{}, "", 10)
	if err != nil {
		t.Fatalf("Failed to filter targets: %v", err)
	}

	if len(filtered) != 2 {
		t.Errorf("Expected 2 targets for example.com, got %d", len(filtered))
	}

	for _, target := range filtered {
		if target.Host != "example.com" {
			t.Errorf("Expected host 'example.com', got '%s'", target.Host)
		}
	}
}

func TestIdempotencyKeyStorage(t *testing.T) {
	store := setupTestDB(t)
	ctx := context.Background()

	key := "test-key-123"
	requestHash := "hash123"
	targetID := "t_123"
	responseCode := 201
	responseBody := map[string]string{"id": "t_123", "url": "https://example.com"}

	// First call should create new entry
	response1, created1, err := store.UpsertIdempotencyKey(ctx, key, requestHash, targetID, responseCode, responseBody)
	if err != nil {
		t.Fatalf("Failed to create idempotency key: %v", err)
	}

	if !created1 {
		t.Error("Expected idempotency key to be created")
	}

	if response1.ResponseCode != responseCode {
		t.Errorf("Expected response code %d, got %d", responseCode, response1.ResponseCode)
	}

	// Second call should return existing entry
	response2, created2, err := store.UpsertIdempotencyKey(ctx, key, requestHash, targetID, responseCode, responseBody)
	if err != nil {
		t.Fatalf("Failed to get existing idempotency key: %v", err)
	}

	if created2 {
		t.Error("Expected idempotency key to already exist")
	}

	if response2.ResponseCode != responseCode {
		t.Errorf("Expected cached response code %d, got %d", responseCode, response2.ResponseCode)
	}

	// Test GetIdempotencyKey method
	response3, found, err := store.GetIdempotencyKey(ctx, key)
	if err != nil {
		t.Fatalf("Failed to get idempotency key: %v", err)
	}

	if !found {
		t.Error("Expected to find existing idempotency key")
	}

	if response3.ResponseCode != responseCode {
		t.Errorf("Expected response code %d, got %d", responseCode, response3.ResponseCode)
	}

	// Test non-existent key
	_, found2, err := store.GetIdempotencyKey(ctx, "non-existent-key")
	if err != nil {
		t.Fatalf("Failed to check non-existent key: %v", err)
	}

	if found2 {
		t.Error("Expected not to find non-existent key")
	}
}

func TestCheckResultStorage(t *testing.T) {
	store := setupTestDB(t)
	ctx := context.Background()

	// Create a target first
	target, _, err := store.UpsertTargetByURL(ctx, "https://example.com", "example.com")
	if err != nil {
		t.Fatalf("Failed to create target: %v", err)
	}

	// Create check results
	results := []*CheckResult{
		{
			TargetID:   target.ID,
			CheckedAt:  time.Now().Add(-2 * time.Hour),
			StatusCode: &[]int{200}[0],
			LatencyMs:  150,
		},
		{
			TargetID:   target.ID,
			CheckedAt:  time.Now().Add(-1 * time.Hour),
			StatusCode: &[]int{404}[0],
			LatencyMs:  200,
		},
		{
			TargetID:  target.ID,
			CheckedAt: time.Now(),
			LatencyMs: 100,
			Error:     &[]string{"connection timeout"}[0],
		},
	}

	// Insert results
	for _, result := range results {
		if err := store.InsertCheckResult(ctx, result); err != nil {
			t.Fatalf("Failed to insert check result: %v", err)
		}
	}

	// Get all results
	allResults, err := store.GetResults(ctx, target.ID, time.Time{}, 10)
	if err != nil {
		t.Fatalf("Failed to get results: %v", err)
	}

	if len(allResults) != 3 {
		t.Errorf("Expected 3 results, got %d", len(allResults))
	}

	// Verify results are ordered by checked_at DESC (most recent first)
	for i := 1; i < len(allResults); i++ {
		if allResults[i-1].CheckedAt.Before(allResults[i].CheckedAt) {
			t.Error("Results should be ordered by checked_at DESC")
		}
	}

	// Test filtering by since parameter
	sinceTime := time.Now().Add(-90 * time.Minute)
	recentResults, err := store.GetResults(ctx, target.ID, sinceTime, 10)
	if err != nil {
		t.Fatalf("Failed to get recent results: %v", err)
	}

	if len(recentResults) != 2 {
		t.Errorf("Expected 2 recent results, got %d", len(recentResults))
	}
}
