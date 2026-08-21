package repositories

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/justinndidit/notificationSystem/orchestrator/internal/models"
	"github.com/rs/zerolog"
)

// OutboxEntry is one message awaiting publication.
type OutboxEntry struct {
	ID             uuid.UUID      `db:"id"`
	NotificationID uuid.UUID      `db:"notification_id"`
	CorrelationID  uuid.UUID      `db:"correlation_id"`
	RoutingKey     string         `db:"routing_key"`
	Payload        models.JSONMap `db:"payload"`
	Status         string         `db:"status"`
	Attempts       int            `db:"attempts"`
	MaxAttempts    int            `db:"max_attempts"`
	LastError      *string        `db:"last_error"`
	AvailableAt    time.Time      `db:"available_at"`
	PublishedAt    *time.Time     `db:"published_at"`
	CreatedAt      time.Time      `db:"created_at"`
}

// Outbox status values.
const (
	OutboxPending   = "pending"
	OutboxPublished = "published"
	OutboxFailed    = "failed"
)

type OutboxRepository struct {
	pool   *pgxpool.Pool
	logger *zerolog.Logger
}

func NewOutboxRepository(pool *pgxpool.Pool, logger *zerolog.Logger) *OutboxRepository {
	return &OutboxRepository{pool: pool, logger: logger}
}

// Pool exposes the connection pool so callers can open a transaction that spans
// both the notifications table and the outbox.
func (r *OutboxRepository) Pool() *pgxpool.Pool {
	return r.pool
}

// EnqueueTx writes an outbox row inside the caller's transaction.
//
// This is the whole point of the pattern: the notification's state change and
// the intent to publish commit together or not at all.
//
// ON CONFLICT DO NOTHING makes re-enrichment safe — a notification that is
// enriched twice (say, after a recovery sweep) cannot produce two messages.
func (r *OutboxRepository) EnqueueTx(
	ctx context.Context,
	tx pgx.Tx,
	notificationID, correlationID uuid.UUID,
	routingKey string,
	payload models.JSONMap,
) error {
	query := `
		INSERT INTO notification_outbox (
			notification_id, correlation_id, routing_key, payload
		) VALUES ($1, $2, $3, $4)
		ON CONFLICT (notification_id) DO NOTHING
	`

	if _, err := tx.Exec(ctx, query, notificationID, correlationID, routingKey, payload); err != nil {
		return fmt.Errorf("failed to enqueue outbox entry: %w", err)
	}

	return nil
}

// ClaimDue locks a batch of due entries for this worker and returns them.
//
// FOR UPDATE SKIP LOCKED lets several orchestrator replicas drain the same
// table without coordinating and without processing the same row twice. The
// caller must commit or roll back the returned transaction.
func (r *OutboxRepository) ClaimDue(ctx context.Context, limit int) (pgx.Tx, []OutboxEntry, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to begin outbox transaction: %w", err)
	}

	query := `
		SELECT id, notification_id, correlation_id, routing_key, payload,
		       status, attempts, max_attempts, last_error,
		       available_at, published_at, created_at
		FROM notification_outbox
		WHERE status = 'pending' AND available_at <= NOW()
		ORDER BY created_at
		LIMIT $1
		FOR UPDATE SKIP LOCKED
	`

	rows, err := tx.Query(ctx, query, limit)
	if err != nil {
		_ = tx.Rollback(ctx)
		return nil, nil, fmt.Errorf("failed to claim outbox entries: %w", err)
	}

	entries := make([]OutboxEntry, 0, limit)
	for rows.Next() {
		var e OutboxEntry
		if err := rows.Scan(
			&e.ID, &e.NotificationID, &e.CorrelationID, &e.RoutingKey, &e.Payload,
			&e.Status, &e.Attempts, &e.MaxAttempts, &e.LastError,
			&e.AvailableAt, &e.PublishedAt, &e.CreatedAt,
		); err != nil {
			rows.Close()
			_ = tx.Rollback(ctx)
			return nil, nil, fmt.Errorf("failed to scan outbox entry: %w", err)
		}
		entries = append(entries, e)
	}
	rows.Close()

	if err := rows.Err(); err != nil {
		_ = tx.Rollback(ctx)
		return nil, nil, fmt.Errorf("failed to iterate outbox entries: %w", err)
	}

	return tx, entries, nil
}

// MarkPublishedTx records a successful publish within the claiming transaction.
func (r *OutboxRepository) MarkPublishedTx(ctx context.Context, tx pgx.Tx, id uuid.UUID) error {
	query := `
		UPDATE notification_outbox
		SET status = 'published',
		    attempts = attempts + 1,
		    published_at = NOW(),
		    last_error = NULL
		WHERE id = $1
	`

	if _, err := tx.Exec(ctx, query, id); err != nil {
		return fmt.Errorf("failed to mark outbox entry published: %w", err)
	}

	return nil
}

// MarkAttemptFailedTx records a failed publish and schedules the next attempt,
// giving up once max_attempts is reached.
func (r *OutboxRepository) MarkAttemptFailedTx(
	ctx context.Context,
	tx pgx.Tx,
	id uuid.UUID,
	reason string,
	backoff time.Duration,
) error {
	query := `
		UPDATE notification_outbox
		SET attempts = attempts + 1,
		    last_error = $2,
		    status = CASE WHEN attempts + 1 >= max_attempts THEN 'failed' ELSE 'pending' END,
		    available_at = NOW() + $3::interval
		WHERE id = $1
	`

	interval := fmt.Sprintf("%d seconds", int(backoff.Seconds()))
	if _, err := tx.Exec(ctx, query, id, reason, interval); err != nil {
		return fmt.Errorf("failed to record outbox failure: %w", err)
	}

	return nil
}

// CountPending reports how many entries are waiting, for health and metrics.
func (r *OutboxRepository) CountPending(ctx context.Context) (pending int64, failed int64, err error) {
	query := `
		SELECT
			COUNT(*) FILTER (WHERE status = 'pending'),
			COUNT(*) FILTER (WHERE status = 'failed')
		FROM notification_outbox
	`

	if err := r.pool.QueryRow(ctx, query).Scan(&pending, &failed); err != nil {
		return 0, 0, fmt.Errorf("failed to count outbox entries: %w", err)
	}

	return pending, failed, nil
}
