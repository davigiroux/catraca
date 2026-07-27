// Package store persists Payment Intents to SQLite.
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

// State is a Payment Intent's lifecycle state.
type State string

const (
	StatePending    State = "pending"
	StateDetected   State = "detected"
	StateConfirmed  State = "confirmed"
	StateExpired    State = "expired"
	StateMismatched State = "mismatched"
)

// ErrNotFound is returned when a lookup finds no matching intent.
var ErrNotFound = errors.New("store: not found")

// Intent is a Payment Intent: a merchant's expectation of one specific
// on-chain transfer.
type Intent struct {
	ID         string
	MerchantID string
	Recipient  string
	Amount     string
	Mint       string // empty means native SOL
	Reference  string
	Deadline   time.Time
	State      State
	CreatedAt  time.Time
}

// NewIntentParams are the fields a caller supplies to create an Intent.
type NewIntentParams struct {
	MerchantID string
	Recipient  string
	Amount     string
	Mint       string // empty means native SOL
	Reference  string
	Deadline   time.Time
}

// Store persists Payment Intents in SQLite.
type Store struct {
	db *sql.DB
}

// Open opens (and migrates) a SQLite-backed Store at path. Use ":memory:"
// for an in-memory database.
func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("store: open: %w", err)
	}
	// SQLite handles a single writer at a time; avoid "database is locked"
	// errors under concurrent access by serializing connections.
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
CREATE TABLE IF NOT EXISTS payment_intents (
	id          TEXT PRIMARY KEY,
	merchant_id TEXT NOT NULL,
	recipient   TEXT NOT NULL,
	amount      TEXT NOT NULL,
	mint        TEXT,
	reference   TEXT NOT NULL,
	deadline    INTEGER NOT NULL,
	state       TEXT NOT NULL,
	created_at  INTEGER NOT NULL
);
`
	_, err := s.db.Exec(schema)
	if err != nil {
		return fmt.Errorf("store: migrate: %w", err)
	}
	return nil
}

// Close closes the underlying database connection.
func (s *Store) Close() error {
	return s.db.Close()
}

// CreateIntent inserts a new Pending Payment Intent and returns it with its
// generated ID and creation time.
func (s *Store) CreateIntent(ctx context.Context, p NewIntentParams) (Intent, error) {
	intent := Intent{
		ID:         uuid.NewString(),
		MerchantID: p.MerchantID,
		Recipient:  p.Recipient,
		Amount:     p.Amount,
		Mint:       p.Mint,
		Reference:  p.Reference,
		Deadline:   p.Deadline.UTC().Truncate(time.Second),
		State:      StatePending,
		CreatedAt:  time.Now().UTC().Truncate(time.Second),
	}

	var mint any
	if intent.Mint != "" {
		mint = intent.Mint
	}

	_, err := s.db.ExecContext(ctx, `
INSERT INTO payment_intents (id, merchant_id, recipient, amount, mint, reference, deadline, state, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		intent.ID, intent.MerchantID, intent.Recipient, intent.Amount, mint,
		intent.Reference, intent.Deadline.Unix(), string(intent.State), intent.CreatedAt.Unix(),
	)
	if err != nil {
		return Intent{}, fmt.Errorf("store: create intent: %w", err)
	}
	return intent, nil
}

// GetIntent fetches a Payment Intent by ID. It returns ErrNotFound if no
// intent with that ID exists.
func (s *Store) GetIntent(ctx context.Context, id string) (Intent, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT id, merchant_id, recipient, amount, mint, reference, deadline, state, created_at
FROM payment_intents WHERE id = ?`, id)

	var (
		intent   Intent
		mint     sql.NullString
		deadline int64
		state    string
		created  int64
	)
	err := row.Scan(&intent.ID, &intent.MerchantID, &intent.Recipient, &intent.Amount,
		&mint, &intent.Reference, &deadline, &state, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return Intent{}, ErrNotFound
	}
	if err != nil {
		return Intent{}, fmt.Errorf("store: get intent: %w", err)
	}

	intent.Mint = mint.String
	intent.Deadline = time.Unix(deadline, 0).UTC()
	intent.State = State(state)
	intent.CreatedAt = time.Unix(created, 0).UTC()
	return intent, nil
}
