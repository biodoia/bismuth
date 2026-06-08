// Package shared contains tiny utilities used across internal packages.
package shared

import (
	"database/sql"
	"encoding/json"
)

// JSONRaw marshals v into a json.RawMessage. Returns nil on error.
func JSONRaw(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	return b
}

// NullString wraps sql.NullString for clean JSON: emits string value
// when Valid, or null when not. Avoids the {"String":"x","Valid":true}
// serialization of the raw sql.NullString.
type NullString sql.NullString

func (ns NullString) MarshalJSON() ([]byte, error) {
	if !ns.Valid {
		return []byte("null"), nil
	}
	return json.Marshal(ns.String)
}

// NullInt64 wraps sql.NullInt64 for clean JSON.
type NullInt64 sql.NullInt64

func (ni NullInt64) MarshalJSON() ([]byte, error) {
	if !ni.Valid {
		return []byte("null"), nil
	}
	return json.Marshal(ni.Int64)
}
