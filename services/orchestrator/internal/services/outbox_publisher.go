package services

import (
	"context"
	"encoding/json"
	"math"
	"time"

	"github.com/justinndidit/notificationSystem/orchestrator/internal/dtos"
	"github.com/justinndidit/notificationSystem/orchestrator/internal/repositories"
)

const (
	// outboxPollInterval is the idle poll rate. A batch that fills is followed
	// immediately by another, so this only governs how fast an idle publisher
	// notices new work.
	outboxPollInterval = 2 * time.Second

	// outboxBatchSize bounds how many entries one pass claims. Each claim holds
	// row locks until the batch commits, so this trades throughput against how
	// long other replicas are kept waiting.
	outboxBatchSize = 50

	outboxBaseBackoff = 5 * time.Second
	outboxMaxBackoff  = 10 * time.Minute
)

// StartOutboxPublisher drains the outbox into RabbitMQ for the life of the
// process.
//
// Publishing is deliberately separate from enrichment: enrichment's job ends
// once the intent to publish is committed. If the broker is down, entries
// accumulate and are retried with backoff rather than being lost with the
// goroutine that produced them.
func (o *Orchestrator) StartOutboxPublisher(ctx context.Context) {
	go func() {
		o.logger.Info().
			Dur("poll_interval", outboxPollInterval).
			Int("batch_size", outboxBatchSize).
			Msg("Outbox publisher started")

		for {
			published, err := o.drainOutboxOnce(ctx)
			if err != nil {
				o.logger.Error().Err(err).Msg("Outbox drain failed")
			}

			// A full batch probably means more work is waiting; go straight back
			// round rather than sleeping through it.
			if published >= outboxBatchSize {
				select {
				case <-ctx.Done():
					o.logger.Info().Msg("Outbox publisher stopped")
					return
				default:
					continue
				}
			}

			select {
			case <-ctx.Done():
				o.logger.Info().Msg("Outbox publisher stopped")
				return
			case <-time.After(outboxPollInterval):
			}
		}
	}()
}

// drainOutboxOnce claims one batch and attempts to publish each entry.
func (o *Orchestrator) drainOutboxOnce(ctx context.Context) (int, error) {
	claimCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	tx, entries, err := o.outboxRepo.ClaimDue(claimCtx, outboxBatchSize)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback(claimCtx) }()

	if len(entries) == 0 {
		return 0, nil
	}

	handled := 0
	for _, entry := range entries {
		var notification dtos.EnrichedNotification

		raw, marshalErr := json.Marshal(entry.Payload)
		if marshalErr == nil {
			marshalErr = json.Unmarshal(raw, &notification)
		}
		if marshalErr != nil {
			// The payload will never become valid, so burn the attempts rather
			// than retrying it forever.
			o.logger.Error().Err(marshalErr).
				Str("outbox_id", entry.ID.String()).
				Str("notification_id", entry.NotificationID.String()).
				Msg("Outbox entry payload is not a valid enriched notification")

			if err := o.outboxRepo.MarkAttemptFailedTx(claimCtx, tx, entry.ID, marshalErr.Error(), outboxMaxBackoff); err != nil {
				return handled, err
			}
			continue
		}

		if err := o.publishToQueue(claimCtx, notification); err != nil {
			backoff := outboxBackoff(entry.Attempts)

			o.logger.Warn().Err(err).
				Str("outbox_id", entry.ID.String()).
				Str("notification_id", entry.NotificationID.String()).
				Int("attempt", entry.Attempts+1).
				Int("max_attempts", entry.MaxAttempts).
				Dur("retry_in", backoff).
				Msg("Failed to publish outbox entry")

			if err := o.outboxRepo.MarkAttemptFailedTx(claimCtx, tx, entry.ID, err.Error(), backoff); err != nil {
				return handled, err
			}
			continue
		}

		if err := o.outboxRepo.MarkPublishedTx(claimCtx, tx, entry.ID); err != nil {
			return handled, err
		}
		handled++
	}

	if err := tx.Commit(claimCtx); err != nil {
		return handled, err
	}

	if handled > 0 {
		o.logger.Info().Int("published", handled).Msg("Drained outbox entries")
	}

	return len(entries), nil
}

// outboxBackoff grows exponentially from outboxBaseBackoff up to outboxMaxBackoff.
func outboxBackoff(attempts int) time.Duration {
	if attempts < 0 {
		attempts = 0
	}

	backoff := time.Duration(float64(outboxBaseBackoff) * math.Pow(2, float64(attempts)))
	if backoff > outboxMaxBackoff || backoff <= 0 {
		return outboxMaxBackoff
	}

	return backoff
}

// OutboxDepth reports pending and permanently failed entries, for health checks
// and, later, metrics.
func (o *Orchestrator) OutboxDepth(ctx context.Context) (pending int64, failed int64, err error) {
	return o.outboxRepo.CountPending(ctx)
}

// Ensure the repository's status constants stay referenced from this package,
// so a rename cannot silently diverge from the CHECK constraint.
var _ = []string{
	repositories.OutboxPending,
	repositories.OutboxPublished,
	repositories.OutboxFailed,
}
