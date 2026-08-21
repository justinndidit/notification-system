package database

import (
	"context"
	"fmt"
	"time"

	"github.com/rs/zerolog"
)

// PartitionMonthsAhead is how far ahead partitions are pre-created. Three months
// gives ample margin: even if the maintainer stops running entirely, inserts
// keep succeeding for roughly a quarter before anyone notices.
const PartitionMonthsAhead = 3

// partitionCheckInterval is how often the window is re-checked. Partitions turn
// over monthly, so this only needs to be frequent enough that a missed run is
// not fatal.
const partitionCheckInterval = 12 * time.Hour

// EnsurePartitions creates any missing monthly partitions for the notifications
// table and reports which ones it added.
func EnsurePartitions(ctx context.Context, db *Database, logger *zerolog.Logger, monthsAhead int) error {
	rows, err := db.Pool.Query(ctx, `SELECT partition_name, created FROM create_notification_partitions($1)`, monthsAhead)
	if err != nil {
		return fmt.Errorf("failed to ensure partitions: %w", err)
	}
	defer rows.Close()

	var created []string
	var total int

	for rows.Next() {
		var name string
		var wasCreated bool
		if err := rows.Scan(&name, &wasCreated); err != nil {
			return fmt.Errorf("failed to read partition result: %w", err)
		}
		total++
		if wasCreated {
			created = append(created, name)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("failed to iterate partition results: %w", err)
	}

	if len(created) > 0 {
		logger.Info().
			Strs("created", created).
			Int("window", total).
			Msg("Created notification partitions")
	} else {
		logger.Debug().Int("window", total).Msg("Notification partitions already present")
	}

	return nil
}

// StartPartitionMaintainer keeps the partition window open for the lifetime of
// the process.
//
// Without this the table only ever has the partitions created at migration
// time, and inserts begin failing outright once the calendar moves past them.
func StartPartitionMaintainer(ctx context.Context, db *Database, logger *zerolog.Logger) {
	go func() {
		ticker := time.NewTicker(partitionCheckInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				logger.Info().Msg("Partition maintainer stopped")
				return
			case <-ticker.C:
				checkCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				if err := EnsurePartitions(checkCtx, db, logger, PartitionMonthsAhead); err != nil {
					// Losing one run is survivable — the window is months wide —
					// so log and wait for the next tick rather than exiting.
					logger.Error().Err(err).Msg("Scheduled partition maintenance failed")
				}
				cancel()
			}
		}
	}()
}
