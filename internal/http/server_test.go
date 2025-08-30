package http

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/you/linkwatch/internal/store"
)

// MockStore implements the Store interface for testing
type MockStore struct {
	targets         map[string]*store.Target
	idempotencyKeys map[string]*store.IdempotencyResponse
	results         map[string][]*store.CheckResult
}

func NewMockStore() *MockStore {
	return &MockStore{
		targets:         make(map[string]*store.Target),
		idempotencyKeys: make(map[string]*store.IdempotencyResponse),
		results:         make(map[string][]*store.CheckResult),
	}
}

func (m *MockStore) UpsertTargetByURL(ctx context.Context, canonicalURL, host string) (*store.Target, bool, error) {
	// Check if target already exists
	for _, target := range m.targets {
		if target.URL == canonicalURL {
			return target, false, nil
		}
	}

	// Create new target
	target := &store.Target{
		ID:        "t_test_123",
		URL:       canonicalURL,
		Host:      host,
		CreatedAt: time.Now(),
	}
	m.targets[target.ID] = target
	return target, true, nil
}

func (m *MockStore) GetTargets(ctx context.Context, hostFilter string, afterCreatedAt time.Time, afterID string, limit int) ([]*store.Target, *store.Cursor, error) {
	var targets []*store.Target
	for _, target := range m.targets {
		if hostFilter == "" || target.Host == hostFilter {
			targets = append(targets, target)
		}
	}
	return targets, nil, nil
}

func (m *MockStore) InsertCheckResult(ctx context.Context, result *store.CheckResult) error {
	if m.results[result.TargetID] == nil {
		m.results[result.TargetID] = []*store.CheckResult{}
	}
	m.results[result.TargetID] = append(m.results[result.TargetID], result)
	return nil
}

func (m *MockStore) GetResults(ctx context.Context, targetID string, since time.Time, limit int) ([]*store.CheckResult, error) {
	return m.results[targetID], nil
}

func (m *MockStore) UpsertIdempotencyKey(ctx context.Context, key, requestHash, targetID string, responseCode int, responseBody interface{}) (*store.IdempotencyResponse, bool, error) {
	if existing, exists := m.idempotencyKeys[key]; exists {
		return existing, false, nil
	}

	response := &store.IdempotencyResponse{
		ResponseCode: responseCode,
		ResponseBody: responseBody,
	}
	m.idempotencyKeys[key] = response
	return response, true, nil
}

func (m *MockStore) GetIdempotencyKey(ctx context.Context, key string) (*store.IdempotencyResponse, bool, error) {
	if response, exists := m.idempotencyKeys[key]; exists {
		return response, true, nil
	}
	return nil, false, nil
}

func TestCreateTargetIdempotency(t *testing.T) {
	mockStore := NewMockStore()
	server := NewServer(mockStore)

	// Test data
	requestBody := `{"url":"https://example.com"}`
	idempotencyKey := "test-key-123"

	// First request
	req1 := httptest.NewRequest("POST", "/v1/targets", bytes.NewBufferString(requestBody))
	req1.Header.Set("Content-Type", "application/json")
	req1.Header.Set("Idempotency-Key", idempotencyKey)

	rr1 := httptest.NewRecorder()
	server.Router().ServeHTTP(rr1, req1)

	// Verify first request
	if rr1.Code != http.StatusCreated {
		t.Errorf("First request: expected status 201, got %d", rr1.Code)
	}

	var firstResponse map[string]interface{}
	if err := json.Unmarshal(rr1.Body.Bytes(), &firstResponse); err != nil {
		t.Fatalf("Failed to parse first response: %v", err)
	}

	// Second request with same idempotency key
	req2 := httptest.NewRequest("POST", "/v1/targets", bytes.NewBufferString(requestBody))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("Idempotency-Key", idempotencyKey)

	rr2 := httptest.NewRecorder()
	server.Router().ServeHTTP(rr2, req2)

	// Verify second request returns cached response
	if rr2.Code != http.StatusOK {
		t.Errorf("Second request: expected status 200, got %d", rr2.Code)
	}

	var secondResponse map[string]interface{}
	if err := json.Unmarshal(rr2.Body.Bytes(), &secondResponse); err != nil {
		t.Fatalf("Failed to parse second response: %v", err)
	}

	// Verify responses have same target data
	if firstResponse["id"] != secondResponse["id"] {
		t.Errorf("Idempotent requests should return same target ID")
	}
	if firstResponse["url"] != secondResponse["url"] {
		t.Errorf("Idempotent requests should return same URL")
	}
}

func TestCreateTargetWithoutIdempotencyKey(t *testing.T) {
	mockStore := NewMockStore()
	server := NewServer(mockStore)

	requestBody := `{"url":"https://example.com"}`

	// First request without idempotency key
	req1 := httptest.NewRequest("POST", "/v1/targets", bytes.NewBufferString(requestBody))
	req1.Header.Set("Content-Type", "application/json")

	rr1 := httptest.NewRecorder()
	server.Router().ServeHTTP(rr1, req1)

	if rr1.Code != http.StatusCreated {
		t.Errorf("Expected status 201, got %d", rr1.Code)
	}

	// Second request without idempotency key should return existing target
	req2 := httptest.NewRequest("POST", "/v1/targets", bytes.NewBufferString(requestBody))
	req2.Header.Set("Content-Type", "application/json")

	rr2 := httptest.NewRecorder()
	server.Router().ServeHTTP(rr2, req2)

	if rr2.Code != http.StatusOK {
		t.Errorf("Second request should return existing target with status 200, got %d", rr2.Code)
	}
}

func TestHealthCheck(t *testing.T) {
	mockStore := NewMockStore()
	server := NewServer(mockStore)

	req := httptest.NewRequest("GET", "/healthz", nil)
	rr := httptest.NewRecorder()

	server.Router().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Health check: expected status 200, got %d", rr.Code)
	}

	var response map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse health check response: %v", err)
	}

	if response["status"] != "ok" {
		t.Errorf("Expected status 'ok', got '%s'", response["status"])
	}
}

func TestListTargets(t *testing.T) {
	mockStore := NewMockStore()
	server := NewServer(mockStore)

	// Create a target first
	requestBody := `{"url":"https://example.com"}`
	req := httptest.NewRequest("POST", "/v1/targets", bytes.NewBufferString(requestBody))
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	server.Router().ServeHTTP(rr, req)

	// List targets
	listReq := httptest.NewRequest("GET", "/v1/targets", nil)
	listRr := httptest.NewRecorder()

	server.Router().ServeHTTP(listRr, listReq)

	if listRr.Code != http.StatusOK {
		t.Errorf("List targets: expected status 200, got %d", listRr.Code)
	}

	var response map[string]interface{}
	if err := json.Unmarshal(listRr.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse list response: %v", err)
	}

	items, ok := response["items"].([]interface{})
	if !ok {
		t.Errorf("Expected 'items' array in response")
	}

	if len(items) != 1 {
		t.Errorf("Expected 1 target, got %d", len(items))
	}
}
