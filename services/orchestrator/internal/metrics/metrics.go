// Package metrics exposes the orchestrator's operational signals to Prometheus.
//
// The set is chosen around the questions an operator actually asks during an
// incident: is work arriving, is it getting through, where is it piling up, and
// how long is it taking. Queue and outbox depth are the leading indicators —
// they move before anything starts failing outright.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

const namespace = "notification"

var (
	// NotificationsReceived counts submissions accepted at the ingress.
	NotificationsReceived = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "received_total",
		Help:      "Notification requests accepted, by channel.",
	}, []string{"channel"})

	// DuplicatesSuppressed counts requests short-circuited by an idempotency
	// claim. A sudden rise usually means a client is retrying in a loop.
	DuplicatesSuppressed = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "duplicates_suppressed_total",
		Help:      "Requests recognised as duplicates by their idempotency key.",
	})

	// EnrichmentTotal counts enrichment outcomes. The stage label on failures
	// says where it broke — user fetch, template fetch, render, recipient.
	EnrichmentTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "enrichment_total",
		Help:      "Enrichment attempts by channel, result and failure stage.",
	}, []string{"channel", "result", "stage"})

	// EnrichmentDuration measures the whole enrichment path, which is dominated
	// by the downstream HTTP calls.
	EnrichmentDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: namespace,
		Name:      "enrichment_duration_seconds",
		Help:      "Time from starting enrichment to committing the outbox entry.",
		// Downstream clients retry for up to 30s, so the upper buckets need to
		// reach well past a single fast call.
		Buckets: []float64{.05, .1, .25, .5, 1, 2.5, 5, 10, 30, 60},
	}, []string{"channel"})

	// PublishTotal counts outbox publish attempts.
	PublishTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "publish_total",
		Help:      "Outbox publish attempts by channel and result.",
	}, []string{"channel", "result"})

	// PublishDuration measures a publish including waiting for the broker's
	// confirmation, so it captures broker slowness rather than just local work.
	PublishDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Namespace: namespace,
		Name:      "publish_duration_seconds",
		Help:      "Time to publish one message and receive its confirmation.",
		Buckets:   []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
	})

	// OutboxDepth is the leading indicator for delivery problems: it rises the
	// moment publishing stalls, well before anything is marked failed.
	OutboxDepth = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "outbox_entries",
		Help:      "Outbox entries by status.",
	}, []string{"status"})

	// StatusTransitions counts lifecycle transitions, including those reported
	// by workers through the status callback.
	StatusTransitions = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "status_transitions_total",
		Help:      "Notification status transitions.",
	}, []string{"status"})

	// RetriesTotal counts explicit requeues, whether operator-initiated or from
	// the recovery sweeper.
	RetriesTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "retries_total",
		Help:      "Notifications requeued, by origin.",
	}, []string{"origin"})

	// RecoveredTotal counts notifications the sweeper found abandoned. This
	// should normally be zero; anything sustained means processes are dying
	// mid-enrichment.
	RecoveredTotal = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "recovered_total",
		Help:      "Notifications picked up after being abandoned during enrichment.",
	})
)

// DependencyUp reflects the last health probe for each dependency: 1 healthy,
// 0 unhealthy. Alerting on this is more useful than parsing health-check logs.
var DependencyUp = promauto.NewGaugeVec(prometheus.GaugeOpts{
	Namespace: namespace,
	Name:      "dependency_up",
	Help:      "Whether a dependency responded to its last health check.",
}, []string{"dependency"})

// Result label values, kept as constants so a typo cannot silently create a new
// time series.
const (
	ResultSuccess   = "success"
	ResultFailure   = "failure"
	ResultCancelled = "cancelled"
)
