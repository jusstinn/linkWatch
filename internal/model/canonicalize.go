package model

import (
	"fmt"
	"net/url"
	"sort"
	"strings"
)

// Rules to apply during canonicalization:
//   - Only http and https schemes allowed
//   - Scheme and host lowercased
//   - Default ports removed
//   - URL fragments  removed
//   - Query parameters re sorted by key
//   - Path normalized (removes empty values, trailing slashes if not root)
func Canonicalize(raw string) (string, string, error) {
	// Parse the input 
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", "", fmt.Errorf("invalid URL: %w", err)
	}

	// Only support HTTP and HTTPS
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", "", fmt.Errorf("unsupported scheme: %s", parsed.Scheme)
	}

	// Normalize scheme and host casing
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	parsed.Host = strings.ToLower(parsed.Host)

	// Remove default ports if present
	if parsed.Scheme == "http" && parsed.Port() == "80" {
		parsed.Host = parsed.Hostname()
	}
	if parsed.Scheme == "https" && parsed.Port() == "443" {
		parsed.Host = parsed.Hostname()
	}

	parsed.Fragment = ""

	// Sort query parameters
	if parsed.RawQuery != "" {
		parsed.RawQuery = sortQueryParams(parsed.RawQuery)
	}

	// Normalize path and remove trailing slashes
	parsed.Path = normalizePath(parsed.Path)

	// Final canonical form
	canonicalURL := parsed.String()
	host := parsed.Host

	return canonicalURL, host, nil
}

func normalizePath(path string) string {
	if path == "" || path == "/" {
		return ""
	}

	if strings.HasSuffix(path, "/") {
		return strings.TrimSuffix(path, "/")
	}

	return path
}

func sortQueryParams(rawQuery string) string {
	values, err := url.ParseQuery(rawQuery)
	if err != nil {
		return rawQuery
	}

	var keys []string
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// Rebuild query string with keys in order
	var pairs []string
	for _, k := range keys {
		for _, v := range values[k] {
			pairs = append(pairs, fmt.Sprintf("%s=%s", k, v))
		}
	}

	return strings.Join(pairs, "&")
}
