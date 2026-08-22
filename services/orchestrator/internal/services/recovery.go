package services

import (
	"context"
	"time"

	"github.com/justinndidit/notificationSystem/orchestrator/internal/dtos"
	"github.com/justinndidit/notificationSystem/orchestrator/internal/metrics"
	"github.com/justinndidit/notificationSystem/orchestrator/internal/models"
)

const (
	// stuckAfter is how long a notification may sit in pending or enriching
	// before it is assumed abandoned. It must comfortably exceed the worst-case
	// enrichment time — the HTTP clients retry for up to 30s per downstream
	// call — or the sweeper will fight healthy in-flight work.
	stuckAfter = 5 * time.Minute

	recoveryInterval  = 1 * time.Minute
	recoveryBatchSize = 20
)

// StartRecoverySweeper re-enriches notifications abandoned mid-flight.
//
// The outbox protects everything after enrichment commits. This covers the
// window before it: a process that dies while calling the user or template
// service leaves a durable notification row that nothing would otherwise pick
// up again.
//
// Re-enrichment is safe to repeat. The outbox is keyed uniquely on
// notification_id, so a notification that is enriched twice still produces at
// most one message.
func (o *Orchestrator) StartRecoverySweeper(ctx context.Context) {
	go func() {
		o.logger.Info().
			Dur("stuck_after", stuckAfter).
			Dur("interval", recoveryInterval).
			Msg("Recovery sweeper started")

		ticker := time.NewTicker(recoveryInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				o.logger.Info().Msg("Recovery sweeper stopped")
				return
			case <-ticker.C:
				if err := o.recoverStuckNotifications(ctx); err != nil {
					o.logger.Error().Err(err).Msg("Recovery sweep failed")
				}
			}
		}
	}()
}

func (o *Orchestrator) recoverStuckNotifications(ctx context.Context) error {
	sweepCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	stuck, err := o.notifRepo.FindStuck(sweepCtx, stuckAfter, recoveryBatchSize)
	if err != nil {
		return err
	}
	if len(stuck) == 0 {
		return nil
	}

	o.logger.Warn().
		Int("count", len(stuck)).
		Msg("Found notifications abandoned during enrichment, re-enriching")

	for i := range stuck {
		notification := stuck[i]

		req, ok := o.reconstructRequest(&notification)
		if !ok {
			// Without the original request there is nothing to retry, so mark it
			// failed rather than sweeping it again every minute forever.
			o.logger.Error().
				Str("notification_id", notification.ID.String()).
				Msg("Cannot reconstruct request for stuck notification, marking failed")

			o.notifRepo.UpdateFailure(sweepCtx, notification.ID, notification.CreatedAt, "UNRECOVERABLE",
				"notification was abandoned and its request could not be reconstructed")
			o.eventRepo.CreateEventSimple(sweepCtx, notification.ID, notification.CorrelationID,
				dtos.EventFailed, models.JSONMap{"stage": "recovery", "error": "unrecoverable"})
			continue
		}

		o.eventRepo.CreateEventSimple(sweepCtx, notification.ID, notification.CorrelationID,
			dtos.EventRetried, models.JSONMap{
				"stage":           "recovery",
				"previous_status": notification.Status,
			})

		idempotencyKey := ""
		if notification.IdempotencyKey != nil {
			idempotencyKey = *notification.IdempotencyKey
		}

		// Detached from the sweep's timeout: enrichment has its own budget and
		// should not be cancelled when this pass finishes.
		metrics.RecoveredTotal.Inc()
		metrics.RetriesTotal.WithLabelValues("recovery").Inc()

		go o.EnrichAndPublish(context.Background(), req, notification.CorrelationID, idempotencyKey)
	}

	return nil
}

// reconstructRequest rebuilds the original submission from the persisted row.
//
// Everything needed was written at creation time, which is what makes the row
// itself the durable unit of work.
func (o *Orchestrator) reconstructRequest(n *models.Notification) (dtos.NotificationRequest, bool) {
	channel := dtos.NotificationType(n.Channel)
	if channel != dtos.Email && channel != dtos.Push {
		return dtos.NotificationRequest{}, false
	}

	req := dtos.NotificationRequest{
		NotificationType: channel,
		UserID:           n.UserID.String(),
		TemplateCode:     n.TemplateID.String(),
		Variables:        n.Variables,
		Priority:         priorityFromString(n.Priority),
		MetaData:         n.Metadata,
	}

	if n.IdempotencyKey != nil {
		req.RequestID = *n.IdempotencyKey
	} else {
		req.RequestID = n.ID.String()
	}

	return req, true
}

// priorityFromString is the inverse of dtos.NotificationPriorityToString.
func priorityFromString(priority string) int {
	switch priority {
	case "low":
		return int(dtos.Low)
	case "high":
		return int(dtos.High)
	case "urgent":
		return int(dtos.Urgent)
	default:
		return int(dtos.Normal)
	}
}
