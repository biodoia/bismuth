// Package audit provides a tamper-evident append-only audit log.
//
// Every row in the `audit_log` table chains to the previous row via a
// SHA-256 hash. Any tampering with historical rows breaks the chain
// and is detectable by Verify().
//
// Usage:
//
//	log, _ := audit.New(db, "my-random-salt")
//	log.Append(ctx, "user:lisergico25", "spawn_agent", "omx-1", payload)
//	ok, brokenAt, err := log.Verify(ctx)
package audit

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
)

// Log is the audit logger.
type Log struct {
	db   *sql.DB
	salt []byte
}

// New creates an audit log.
func New(db *sql.DB, salt string) (*Log, error) {
	if salt == "" {
		return nil, fmt.Errorf("salt must not be empty")
	}
	return &Log{db: db, salt: []byte(salt)}, nil
}

// Append writes a new row chained to the previous one.
func (l *Log) Append(ctx context.Context, actor, action, target string, payload []byte) error {
	tx, err := l.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var prevHash string
	row := tx.QueryRowContext(ctx, `SELECT row_hash FROM audit_log ORDER BY seq DESC LIMIT 1`)
	if err := row.Scan(&prevHash); err != nil && err != sql.ErrNoRows {
		return err
	}
	if err == sql.ErrNoRows {
		prevHash = "GENESIS"
	}

	h := sha256.New()
	h.Write(l.salt)
	h.Write([]byte("|"))
	h.Write([]byte(prevHash))
	h.Write([]byte("|"))
	h.Write([]byte(actor))
	h.Write([]byte("|"))
	h.Write([]byte(action))
	h.Write([]byte("|"))
	h.Write([]byte(target))
	h.Write([]byte("|"))
	h.Write(payload)
	rowHash := hex.EncodeToString(h.Sum(nil))

	_, err = tx.ExecContext(ctx,
		`INSERT INTO audit_log(actor, action, target, payload, prev_hash, row_hash) VALUES(?,?,?,?,?,?)`,
		actor, action, target, string(payload), prevHash, rowHash)
	if err != nil {
		return err
	}
	return tx.Commit()
}

// Entry is one audit row, newest-first when returned by Recent.
type Entry struct {
	Seq     int64  `json:"seq"`
	TS      string `json:"ts"`
	Actor   string `json:"actor"`
	Action  string `json:"action"`
	Target  string `json:"target"`
	Payload string `json:"payload"`
	RowHash string `json:"row_hash"`
}

// Recent returns audit entries newest-first for the trail UI.
func (l *Log) Recent(ctx context.Context, limit, offset int) ([]Entry, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	rows, err := l.db.QueryContext(ctx,
		`SELECT seq, ts, actor, action, target, COALESCE(payload,''), row_hash
		 FROM audit_log ORDER BY seq DESC LIMIT ? OFFSET ?`, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Entry
	for rows.Next() {
		var e Entry
		if err := rows.Scan(&e.Seq, &e.TS, &e.Actor, &e.Action, &e.Target, &e.Payload, &e.RowHash); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// Verify walks the chain and confirms every row_hash matches its content.
// Returns (true, 0, nil) if all good, or (false, brokenSeq, err) on tamper.
func (l *Log) Verify(ctx context.Context) (bool, int64, error) {
	rows, err := l.db.QueryContext(ctx, `SELECT seq, actor, action, target, payload, prev_hash, row_hash FROM audit_log ORDER BY seq ASC`)
	if err != nil {
		return false, 0, err
	}
	defer rows.Close()

	var prevHash = "GENESIS"
	var seq int64
	for rows.Next() {
		var (
			aSeq    int64
			actor   string
			action  string
			target  string
			payload string
			rowPrev string
			rowHash string
		)
		if err := rows.Scan(&aSeq, &actor, &action, &target, &payload, &rowPrev, &rowHash); err != nil {
			return false, 0, err
		}
		seq = aSeq
		if rowPrev != prevHash {
			return false, seq, fmt.Errorf("chain break at seq %d: prev_hash mismatch", seq)
		}
		h := sha256.New()
		h.Write(l.salt)
		h.Write([]byte("|"))
		h.Write([]byte(rowPrev))
		h.Write([]byte("|"))
		h.Write([]byte(actor))
		h.Write([]byte("|"))
		h.Write([]byte(action))
		h.Write([]byte("|"))
		h.Write([]byte(target))
		h.Write([]byte("|"))
		h.Write([]byte(payload))
		expected := hex.EncodeToString(h.Sum(nil))
		if expected != rowHash {
			return false, seq, fmt.Errorf("hash mismatch at seq %d", seq)
		}
		prevHash = rowHash
	}
	return true, 0, nil
}
