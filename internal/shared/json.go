// Package shared contains tiny utilities used across internal packages.
package shared

import "encoding/json"

// JSONRaw marshals v into a json.RawMessage. Returns nil on error.
func JSONRaw(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	return b
}
