package voice

import "os"

// osGetenv is a tiny indirection so tests can override.
func osGetenv(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
