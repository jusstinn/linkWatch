package model

import (
	"testing"
)

func TestCanonicalize(t *testing.T) {
	tests := []struct {
		input    string
		expected string
		host     string
	}{
		{"https://EXAMPLE.com/", "https://example.com", "example.com"},
		{"http://example.com:80/", "http://example.com", "example.com"},
		{"https://example.com:443/", "https://example.com", "example.com"},
		{"https://example.com/path/", "https://example.com/path", "example.com"},
		{"https://example.com?b=2&a=1", "https://example.com?a=1&b=2", "example.com"},
	}

	for _, test := range tests {
		canonical, host, err := Canonicalize(test.input)
		if err != nil {
			t.Errorf("Canonicalize(%q) failed: %v", test.input, err)
			continue
		}

		if canonical != test.expected {
			t.Errorf("Canonicalize(%q) = %q, want %q", test.input, canonical, test.expected)
		}

		if host != test.host {
			t.Errorf("Canonicalize(%q) host = %q, want %q", test.input, host, test.host)
		}
	}
}
