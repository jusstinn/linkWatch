package http

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/you/linkwatch/internal/model"
	"github.com/you/linkwatch/internal/store"
)

// Server handles HTTP requests
type Server struct {
	store  store.Store
	router *chi.Mux
}

// NewServer creates HTTP server with routes
func NewServer(store store.Store) *Server {
	s := &Server{store: store}
	s.setupRoutes()
	return s
}

// setupRoutes configures all endpoints
func (s *Server) setupRoutes() {
	s.router = chi.NewRouter()

	s.router.Use(middleware.RequestID)
	s.router.Use(middleware.Logger)
	s.router.Use(middleware.Recoverer)

	s.router.Route("/v1", func(r chi.Router) {
		r.Route("/targets", func(r chi.Router) {
			r.Post("/", s.createTarget)
			r.Get("/", s.listTargets)
			r.Get("/{targetID}/results", s.getResults)
		})
	})

	s.router.Get("/healthz", s.healthCheck)
}

func (s *Server) Router() *chi.Mux {
	return s.router
}

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// createTarget handles POST /v1/targets
func (s *Server) createTarget(w http.ResponseWriter, r *http.Request) {
	var req struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	canonicalURL, host, err := model.Canonicalize(req.URL)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid URL: "+err.Error())
		return
	}
	idempotencyKey := r.Header.Get("Idempotency-Key")
	if idempotencyKey != "" {
		if cachedResponse, found, err := s.checkIdempotencyKey(r.Context(), idempotencyKey, req.URL, canonicalURL); err != nil {
			writeError(w, http.StatusInternalServerError, "idempotency check failed: "+err.Error())
			return
		} else if found {
			writeJSON(w, http.StatusOK, cachedResponse.ResponseBody)
			return
		}
	}
	target, created, err := s.store.UpsertTargetByURL(r.Context(), canonicalURL, host)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "database error: "+err.Error())
		return
	}

	status := http.StatusCreated
	if !created {
		status = http.StatusOK
	}

	if idempotencyKey != "" {
		if err := s.storeIdempotencyResult(r.Context(), idempotencyKey, req.URL, target.ID, status, target); err != nil {
			fmt.Printf("Failed to store idempotency result: %v\n", err)
		}
	}

	writeJSON(w, status, target)
}

// listTargets handles GET /v1/targets
func (s *Server) listTargets(w http.ResponseWriter, r *http.Request) {
	host := r.URL.Query().Get("host")
	limitParam := r.URL.Query().Get("limit")
	pageToken := r.URL.Query().Get("page_token")

	limit := 20
	if limitParam != "" {
		if parsed, err := parseInt(limitParam, 1, 100); err == nil {
			limit = parsed
		}
	}

	afterTime, afterID := parseCursorToken(pageToken)

	targets, cursor, err := s.store.GetTargets(r.Context(), host, afterTime, afterID, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to fetch targets: "+err.Error())
		return
	}

	response := map[string]interface{}{
		"items": targets,
	}

	if cursor != nil {
		response["next_page_token"] = buildCursorToken(cursor.CreatedAt, cursor.ID)
	} else {
		response["next_page_token"] = ""
	}

	writeJSON(w, http.StatusOK, response)
}

// getResults handles GET /v1/targets/{targetID}/results
func (s *Server) getResults(w http.ResponseWriter, r *http.Request) {
	targetID := chi.URLParam(r, "targetID")
	if targetID == "" {
		writeError(w, http.StatusBadRequest, "target ID is required")
		return
	}

	sinceParam := r.URL.Query().Get("since")
	limitParam := r.URL.Query().Get("limit")

	var since time.Time
	if sinceParam != "" {
		if parsed, err := time.Parse(time.RFC3339, sinceParam); err == nil {
			since = parsed
		} else {
			writeError(w, http.StatusBadRequest, "invalid since timestamp format, use RFC3339")
			return
		}
	}

	limit := 50
	if limitParam != "" {
		if parsed, err := parseInt(limitParam, 1, 200); err == nil {
			limit = parsed
		}
	}

	results, err := s.store.GetResults(r.Context(), targetID, since, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to fetch results: "+err.Error())
		return
	}

	response := map[string]interface{}{
		"items": results,
	}

	writeJSON(w, http.StatusOK, response)
}

func (s *Server) healthCheck(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func parseInt(s string, min, max int) (int, error) {
	val, err := strconv.Atoi(s)
	if err != nil {
		return 0, err
	}
	if val < min || val > max {
		return 0, fmt.Errorf("value must be between %d and %d", min, max)
	}
	return val, nil
}

func parseCursorToken(token string) (time.Time, string) {
	if token == "" {
		return time.Time{}, ""
	}

	decoded, err := base64.URLEncoding.DecodeString(token)
	if err != nil {
		return time.Time{}, ""
	}

	parts := strings.Split(string(decoded), "|")
	if len(parts) != 2 {
		return time.Time{}, ""
	}

	createdAt, err := time.Parse(time.RFC3339, parts[0])
	if err != nil {
		return time.Time{}, ""
	}

	return createdAt, parts[1]
}

func buildCursorToken(createdAt time.Time, id string) string {
	token := fmt.Sprintf("%s|%s", createdAt.Format(time.RFC3339), id)
	return base64.URLEncoding.EncodeToString([]byte(token))
}

func (s *Server) checkIdempotencyKey(ctx context.Context, key, requestURL, targetURL string) (*store.IdempotencyResponse, bool, error) {
	return s.store.GetIdempotencyKey(ctx, key)
}

func (s *Server) storeIdempotencyResult(ctx context.Context, key, requestURL, targetID string, responseCode int, responseBody interface{}) error {
	requestHash := createRequestHash(requestURL)
	_, _, err := s.store.UpsertIdempotencyKey(ctx, key, requestHash, targetID, responseCode, responseBody)
	return err
}

func createRequestHash(requestURL string) string {
	hash := sha256.Sum256([]byte(requestURL))
	return fmt.Sprintf("%x", hash)
}
