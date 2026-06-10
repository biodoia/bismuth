package sharedmem

import (
	"context"
	"errors"
)

// NewFallback combines two providers into one.
//
// Post writes to both providers: the fallback write is always attempted,
// even when the primary write fails, and the primary's error decides the
// result (a fallback-only failure is treated as a best-effort replica
// miss and does not fail the call). Query and List are served by the
// primary; the fallback is consulted only when the primary errors. If
// both error, the returned error joins both for diagnostics.
func NewFallback(primary, fallback Provider) Provider {
	return &fallbackProvider{primary: primary, fallback: fallback}
}

// fallbackProvider chains a primary Provider with a fallback Provider.
type fallbackProvider struct {
	primary  Provider
	fallback Provider
}

var _ Provider = (*fallbackProvider)(nil)

// Post writes the memory to both providers. The primary's error wins.
func (f *fallbackProvider) Post(ctx context.Context, m *Memory) error {
	perr := f.primary.Post(ctx, m)
	// Best-effort replica write; attempted even when the primary failed.
	_ = f.fallback.Post(ctx, m)
	return perr
}

// Query tries the primary and falls back on error.
func (f *fallbackProvider) Query(ctx context.Context, q string, limit int) ([]*Memory, error) {
	res, perr := f.primary.Query(ctx, q, limit)
	if perr == nil {
		return res, nil
	}
	res, ferr := f.fallback.Query(ctx, q, limit)
	if ferr != nil {
		return nil, errors.Join(perr, ferr)
	}
	return res, nil
}

// List tries the primary and falls back on error.
func (f *fallbackProvider) List(ctx context.Context, agentID string, limit int) ([]*Memory, error) {
	res, perr := f.primary.List(ctx, agentID, limit)
	if perr == nil {
		return res, nil
	}
	res, ferr := f.fallback.List(ctx, agentID, limit)
	if ferr != nil {
		return nil, errors.Join(perr, ferr)
	}
	return res, nil
}
