package notify

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

// Delivery tracks the state of webhook delivery attempts for a single
// intent. It is stored entirely separately from payment_intents: Delivery
// state never feeds back into an intent's own state.
type Delivery struct {
	IntentID     string
	Attempts     int
	NextRetryAt  time.Time
	LastError    string
	Delivered    bool
	DeadLettered bool
	UpdatedAt    time.Time
}

// Store persists webhook Delivery records in their own table
// (webhook_deliveries), independent of the payment_intents table.
type Store struct {
	db *sql.DB
}

// Open opens (and migrates) a SQLite-backed delivery Store at path. Use
// ":memory:" for an in-memory database. path may be the same file used by
// store.Open — the tables are independent — or a separate file.
func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("notify: open: %w", err)
	}
	db.SetMaxOpenConns(1)

	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		if closeErr := db.Close(); closeErr != nil {
			return nil, fmt.Errorf("%w (also failed to close: %v)", err, closeErr)
		}
		return nil, err
	}
	return s, nil
}

func (s *Store) migrate() error {
	const schema = `
CREATE TABLE IF NOT EXISTS webhook_deliveries (
	intent_id      TEXT PRIMARY KEY,
	attempts       INTEGER NOT NULL DEFAULT 0,
	next_retry_at  INTEGER NOT NULL DEFAULT 0,
	last_error     TEXT NOT NULL DEFAULT '',
	delivered      INTEGER NOT NULL DEFAULT 0,
	dead_lettered  INTEGER NOT NULL DEFAULT 0,
	updated_at     INTEGER NOT NULL DEFAULT 0
);
`
	_, err := s.db.Exec(schema)
	if err != nil {
		return fmt.Errorf("notify: migrate: %w", err)
	}
	return nil
}

// Close closes the underlying database connection.
func (s *Store) Close() error {
	return s.db.Close()
}

// Get returns the Delivery record for intentID, or a zero-attempt Delivery
// (due immediately) if none exists yet.
func (s *Store) Get(ctx context.Context, intentID string) (Delivery, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT intent_id, attempts, next_retry_at, last_error, delivered, dead_lettered, updated_at
FROM webhook_deliveries WHERE intent_id = ?`, intentID)

	var (
		d          Delivery
		nextRetry  int64
		delivered  int
		deadLetter int
		updatedAt  int64
	)
	err := row.Scan(&d.IntentID, &d.Attempts, &nextRetry, &d.LastError, &delivered, &deadLetter, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Delivery{IntentID: intentID}, nil
	}
	if err != nil {
		return Delivery{}, fmt.Errorf("notify: get delivery: %w", err)
	}
	d.NextRetryAt = time.Unix(nextRetry, 0).UTC()
	d.Delivered = delivered != 0
	d.DeadLettered = deadLetter != 0
	d.UpdatedAt = time.Unix(updatedAt, 0).UTC()
	return d, nil
}

// Save upserts a Delivery record.
func (s *Store) Save(ctx context.Context, d Delivery) error {
	delivered := 0
	if d.Delivered {
		delivered = 1
	}
	deadLetter := 0
	if d.DeadLettered {
		deadLetter = 1
	}
	updatedAt := d.UpdatedAt
	if updatedAt.IsZero() {
		updatedAt = time.Now().UTC()
	}

	_, err := s.db.ExecContext(ctx, `
INSERT INTO webhook_deliveries (intent_id, attempts, next_retry_at, last_error, delivered, dead_lettered, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(intent_id) DO UPDATE SET
	attempts = excluded.attempts,
	next_retry_at = excluded.next_retry_at,
	last_error = excluded.last_error,
	delivered = excluded.delivered,
	dead_lettered = excluded.dead_lettered,
	updated_at = excluded.updated_at`,
		d.IntentID, d.Attempts, d.NextRetryAt.Unix(), d.LastError, delivered, deadLetter, updatedAt.Unix(),
	)
	if err != nil {
		return fmt.Errorf("notify: save delivery: %w", err)
	}
	return nil
}
