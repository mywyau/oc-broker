package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/oc/oc-broker/internal/event"
)

var (
	ErrNotFound         = errors.New("not found")
	ErrInactiveClaimant = errors.New("claimant is inactive")
	ErrEmailExists      = errors.New("email already exists")
)

type Store struct {
	Pool *pgxpool.Pool
}

type Claimant struct {
	ID        uuid.UUID `json:"id"`
	FullName  string    `json:"fullName"`
	Email     string    `json:"email"`
	Active    bool      `json:"active"`
	CreatedAt time.Time `json:"createdAt"`
}

type Claim struct {
	ID             uuid.UUID `json:"id"`
	ClaimantID     uuid.UUID `json:"claimantId"`
	PolicyNumber   string    `json:"policyNumber"`
	IncidentDate   time.Time `json:"incidentDate"`
	AmountCents    int64     `json:"amountCents"`
	Currency       string    `json:"currency"`
	Description    string    `json:"description"`
	Status         string    `json:"status"`
	DecisionReason *string   `json:"decisionReason,omitempty"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

type NewClaim struct {
	ClaimantID     uuid.UUID
	PolicyNumber   string
	IncidentDate   time.Time
	AmountCents    int64
	Currency       string
	Description    string
	IdempotencyKey string
}

type OutboxEvent struct {
	EventID       uuid.UUID
	EventType     string
	EventVersion  int
	AggregateID   uuid.UUID
	TargetQueue   string
	CorrelationID uuid.UUID
	Payload       json.RawMessage
	OccurredAt    time.Time
}

func Open(ctx context.Context, databaseURL string) (*Store, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("configure postgres: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("connect postgres: %w", err)
	}
	return &Store{Pool: pool}, nil
}

func (s *Store) Close() { s.Pool.Close() }

func (s *Store) CreateClaimant(ctx context.Context, fullName, email string) (Claimant, error) {
	c := Claimant{ID: uuid.New()}
	err := s.Pool.QueryRow(ctx, `
		INSERT INTO claimants (id, full_name, email)
		VALUES ($1, $2, $3)
		RETURNING full_name, email, active, created_at`, c.ID, strings.TrimSpace(fullName), strings.ToLower(strings.TrimSpace(email))).
		Scan(&c.FullName, &c.Email, &c.Active, &c.CreatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "claimants_email_unique" {
			return Claimant{}, ErrEmailExists
		}
		return Claimant{}, fmt.Errorf("insert claimant: %w", err)
	}
	return c, nil
}

func (s *Store) GetClaimant(ctx context.Context, id uuid.UUID) (Claimant, error) {
	var c Claimant
	err := s.Pool.QueryRow(ctx, `SELECT id, full_name, email, active, created_at FROM claimants WHERE id=$1`, id).
		Scan(&c.ID, &c.FullName, &c.Email, &c.Active, &c.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Claimant{}, ErrNotFound
	}
	return c, err
}

func (s *Store) CreateClaim(ctx context.Context, input NewClaim) (Claim, bool, error) {
	tx, err := s.Pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Claim{}, false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if existing, err := getClaimByIdempotencyKey(ctx, tx, input.IdempotencyKey); err == nil {
		return existing, false, nil
	} else if !errors.Is(err, ErrNotFound) {
		return Claim{}, false, err
	}

	var active bool
	if err := tx.QueryRow(ctx, `SELECT active FROM claimants WHERE id=$1`, input.ClaimantID).Scan(&active); errors.Is(err, pgx.ErrNoRows) {
		return Claim{}, false, ErrNotFound
	} else if err != nil {
		return Claim{}, false, err
	} else if !active {
		return Claim{}, false, ErrInactiveClaimant
	}

	now := time.Now().UTC()
	c := Claim{ID: uuid.New(), ClaimantID: input.ClaimantID, PolicyNumber: input.PolicyNumber, IncidentDate: input.IncidentDate, AmountCents: input.AmountCents, Currency: input.Currency, Description: input.Description, Status: "PENDING", CreatedAt: now, UpdatedAt: now}
	tag, err := tx.Exec(ctx, `
		INSERT INTO claims (id, claimant_id, policy_number, incident_date, amount_cents, currency, description, status, idempotency_key, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$10)
		ON CONFLICT (idempotency_key) DO NOTHING`,
		c.ID, c.ClaimantID, c.PolicyNumber, c.IncidentDate, c.AmountCents, c.Currency, c.Description, c.Status, input.IdempotencyKey, now)
	if err != nil {
		return Claim{}, false, fmt.Errorf("insert claim: %w", err)
	}
	if tag.RowsAffected() == 0 {
		existing, err := getClaimByIdempotencyKey(ctx, tx, input.IdempotencyKey)
		return existing, false, err
	}
	payload, _ := json.Marshal(event.ClaimSubmittedPayload{ClaimID: c.ID})
	if err := insertOutbox(ctx, tx, OutboxEvent{EventID: uuid.New(), EventType: event.ClaimSubmitted, EventVersion: 1, AggregateID: c.ID, TargetQueue: "claim-commands", CorrelationID: c.ID, Payload: payload, OccurredAt: now}); err != nil {
		return Claim{}, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Claim{}, false, err
	}
	return c, true, nil
}

func getClaimByIdempotencyKey(ctx context.Context, tx pgx.Tx, key string) (Claim, error) {
	return scanClaim(tx.QueryRow(ctx, claimColumns+` WHERE idempotency_key=$1`, key))
}

const claimColumns = `SELECT id, claimant_id, policy_number, incident_date, amount_cents, currency, description, status, decision_reason, created_at, updated_at FROM claims`

type rowScanner interface{ Scan(...any) error }

func scanClaim(row rowScanner) (Claim, error) {
	var c Claim
	err := row.Scan(&c.ID, &c.ClaimantID, &c.PolicyNumber, &c.IncidentDate, &c.AmountCents, &c.Currency, &c.Description, &c.Status, &c.DecisionReason, &c.CreatedAt, &c.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Claim{}, ErrNotFound
	}
	return c, err
}

func (s *Store) GetClaim(ctx context.Context, id uuid.UUID) (Claim, error) {
	return scanClaim(s.Pool.QueryRow(ctx, claimColumns+` WHERE id=$1`, id))
}

func (s *Store) ListClaims(ctx context.Context, claimantID uuid.UUID) ([]Claim, error) {
	rows, err := s.Pool.Query(ctx, claimColumns+` WHERE claimant_id=$1 ORDER BY created_at DESC LIMIT 100`, claimantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	claims := make([]Claim, 0)
	for rows.Next() {
		c, err := scanClaim(rows)
		if err != nil {
			return nil, err
		}
		claims = append(claims, c)
	}
	return claims, rows.Err()
}

func insertOutbox(ctx context.Context, tx pgx.Tx, e OutboxEvent) error {
	_, err := tx.Exec(ctx, `INSERT INTO outbox_events (event_id,event_type,event_version,aggregate_id,target_queue,correlation_id,payload,occurred_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`, e.EventID, e.EventType, e.EventVersion, e.AggregateID, e.TargetQueue, e.CorrelationID, e.Payload, e.OccurredAt)
	return err
}

func (s *Store) PendingOutbox(ctx context.Context, limit int) ([]OutboxEvent, error) {
	rows, err := s.Pool.Query(ctx, `SELECT event_id,event_type,event_version,aggregate_id,target_queue,correlation_id,payload,occurred_at FROM outbox_events WHERE published_at IS NULL ORDER BY occurred_at LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	events := make([]OutboxEvent, 0)
	for rows.Next() {
		var e OutboxEvent
		if err := rows.Scan(&e.EventID, &e.EventType, &e.EventVersion, &e.AggregateID, &e.TargetQueue, &e.CorrelationID, &e.Payload, &e.OccurredAt); err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	return events, rows.Err()
}

func (s *Store) MarkPublished(ctx context.Context, id uuid.UUID) error {
	_, err := s.Pool.Exec(ctx, `UPDATE outbox_events SET published_at=now(), attempts=attempts+1, last_error=NULL WHERE event_id=$1`, id)
	return err
}

func (s *Store) MarkPublishFailed(ctx context.Context, id uuid.UUID, cause error) {
	_, _ = s.Pool.Exec(ctx, `UPDATE outbox_events SET attempts=attempts+1, last_error=$2 WHERE event_id=$1`, id, cause.Error())
}

func (s *Store) ProcessSubmittedClaim(ctx context.Context, envelope event.Envelope) error {
	var payload event.ClaimSubmittedPayload
	if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
		return fmt.Errorf("decode payload: %w", err)
	}
	if payload.ClaimID == uuid.Nil {
		return errors.New("claimId is required")
	}

	tx, err := s.Pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	tag, err := tx.Exec(ctx, `INSERT INTO inbox_messages (event_id) VALUES ($1) ON CONFLICT DO NOTHING`, envelope.EventID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return tx.Commit(ctx)
	}

	var amount int64
	var status string
	if err := tx.QueryRow(ctx, `SELECT amount_cents,status FROM claims WHERE id=$1 FOR UPDATE`, payload.ClaimID).Scan(&amount, &status); errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	} else if err != nil {
		return err
	}
	if status != "PENDING" {
		return tx.Commit(ctx)
	}

	newStatus, reason := "APPROVED", ""
	if amount > 1_000_000 {
		newStatus, reason = "REVIEW_REQUIRED", "claims above 10000 major currency units require manual review"
	}
	_, err = tx.Exec(ctx, `UPDATE claims SET status=$2,decision_reason=NULLIF($3,''),updated_at=now() WHERE id=$1`, payload.ClaimID, newStatus, reason)
	if err != nil {
		return err
	}
	outPayload, _ := json.Marshal(event.ClaimProcessedPayload{ClaimID: payload.ClaimID, Status: newStatus, Reason: reason})
	err = insertOutbox(ctx, tx, OutboxEvent{EventID: uuid.New(), EventType: event.ClaimProcessed, EventVersion: 1, AggregateID: payload.ClaimID, TargetQueue: "claim-events", CorrelationID: envelope.CorrelationID, Payload: outPayload, OccurredAt: time.Now().UTC()})
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}
