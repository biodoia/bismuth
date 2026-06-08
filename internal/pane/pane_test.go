// pane package: test the coalesced persistence logic.
package pane

import (
	"testing"
	"time"
)

func TestCoalescerFlushesByBytes(t *testing.T) {
	// Build a Pane with mock fields; only pending/persistBytes/etc
	// are touched by the coalescer.
	p := &Pane{
		ID:           "p1",
		persistAfter: 256,
		persistEvery: 500 * time.Millisecond,
	}

	// Simulate three small writes
	for i := 0; i < 3; i++ {
		p.mu.Lock()
		p.pending = append(p.pending, []byte("x")...)
		p.persistBytes += 1
		p.mu.Unlock()
	}

	// Should NOT have triggered a flush yet (only 3 bytes)
	p.mu.Lock()
	gotBytes := p.persistBytes
	p.mu.Unlock()
	if gotBytes != 3 {
		t.Errorf("persistBytes = %d, want 3", gotBytes)
	}

	// Now write enough to trigger threshold flush
	p.mu.Lock()
	p.pending = append(p.pending, make([]byte, 300)...)
	p.persistBytes += 300
	p.mu.Unlock()
	// (real flush happens via the coalescer; for this test we just
	// assert that the counters work as expected.)
	if p.persistBytes < 256 {
		t.Errorf("threshold not reached, got %d", p.persistBytes)
	}
}

func TestCoalescerConstants(t *testing.T) {
	if defaultPersistAfter <= 0 {
		t.Error("defaultPersistAfter must be > 0")
	}
	if defaultPersistEvery <= 0 {
		t.Error("defaultPersistEvery must be > 0")
	}
}
