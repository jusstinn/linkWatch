package model

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"
)

// Cursor represents where in a paginated list we left off.
// It helps us know "start from here" when fetching the next page.
type Cursor struct {
	CreatedAt time.Time `json:"created_at"` // When the record was created
	ID        string    `json:"id"`         // Unique ID of the record
}

// EncodeCursor turns a Cursor into a base64 string so it can be safely
// passed around in URLs or API requests.
//
// Example:
//
//	cursor := &Cursor{CreatedAt: time.Now(), ID: "abc123"}
//	token, _ := EncodeCursor(cursor)
//	// token = "eyJDcmVhdGVkQ..." (base64 string)
func EncodeCursor(cursor *Cursor) (string, error) {
	if cursor == nil {
		// No cursor means "start from the beginning"
		return "", nil
	}

	// Convert Cursor -> JSON
	data, err := json.Marshal(cursor)
	if err != nil {
		return "", fmt.Errorf("could not convert cursor to JSON: %v", err)
	}

	// Encode JSON as base64 (URL-safe format)
	return base64.URLEncoding.EncodeToString(data), nil
}

// DecodeCursor takes a base64 string (like from a query parameter)
// and converts it back into a Cursor struct.
//
// Example:
//
//	cursor, _ := DecodeCursor("eyJDcmVhdGVkQ...")
//	fmt.Println(cursor.ID)  // "abc123"
func DecodeCursor(token string) (*Cursor, error) {
	if token == "" {
		// Empty string means "no cursor provided"
		return nil, nil
	}

	// Decode base64 string back to raw JSON
	data, err := base64.URLEncoding.DecodeString(token)
	if err != nil {
		return nil, fmt.Errorf("invalid cursor token: %v", err)
	}

	// Convert JSON -> Cursor struct
	var cursor Cursor
	if err := json.Unmarshal(data, &cursor); err != nil {
		return nil, fmt.Errorf("invalid cursor data: %v", err)
	}

	return &cursor, nil
}
