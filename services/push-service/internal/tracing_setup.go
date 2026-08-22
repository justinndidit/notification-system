// Package tracing wires OpenTelemetry into the orchestrator.
//
// Correlation IDs already thread through the logs and every AMQP message.
// Tracing joins those points into a single causal chain, which matters most
// where a request crosses a boundary the logs cannot follow on their own: the
// HTTP calls to the user and template services, and the queue hop to a worker.
package internal

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"
	"go.opentelemetry.io/otel/trace"
)

// ServiceName identifies this service in the trace backend.
const TracingServiceName = "push-service"

// Tracer is the instrumentation handle used across the service.
func Tracer() trace.Tracer {
	return otel.Tracer(TracingServiceName)
}

// Init configures the global tracer provider and propagator.
//
// Returns a shutdown function that flushes pending spans. Tracing is optional:
// when OTEL_EXPORTER_OTLP_ENDPOINT is unset the service runs untraced rather
// than failing, so a missing collector never takes down notification delivery.
func InitTracing(ctx context.Context, logger *zerolog.Logger) (func(context.Context) error, error) {
	endpoint := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	if endpoint == "" {
		logger.Info().Msg("Tracing disabled: OTEL_EXPORTER_OTLP_ENDPOINT is not set")

		// A no-op propagator would break context flow, so still install the
		// real one — spans simply go nowhere.
		otel.SetTextMapPropagator(propagator())
		return func(context.Context) error { return nil }, nil
	}

	// WithEndpointURL takes the full signal URL, not the base: given only
	// "http://collector:4318" it would POST to "/" and every span would be
	// silently discarded by the collector.
	exporter, err := otlptracehttp.New(ctx,
		otlptracehttp.WithEndpointURL(strings.TrimSuffix(endpoint, "/")+"/v1/traces"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create the OTLP exporter: %w", err)
	}

	res, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName(TracingServiceName),
			semconv.ServiceVersion(version()),
			attribute.String("deployment.environment", environment()),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to build the trace resource: %w", err)
	}

	provider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter, sdktrace.WithBatchTimeout(5*time.Second)),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sampler()),
	)

	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(propagator())

	logger.Info().
		Str("endpoint", endpoint).
		Str("service", TracingServiceName).
		Msg("Tracing enabled")

	return provider.Shutdown, nil
}

// propagator handles both W3C trace context and baggage, which is what lets a
// trace survive an HTTP hop and an AMQP hop alike.
func propagator() propagation.TextMapPropagator {
	return propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	)
}

// sampler defaults to sampling everything, which is right for a system at this
// volume. OTEL_TRACES_SAMPLER_ARG sets a ratio when that stops being true.
func sampler() sdktrace.Sampler {
	ratio := os.Getenv("OTEL_TRACES_SAMPLER_ARG")
	if ratio == "" {
		return sdktrace.AlwaysSample()
	}

	var parsed float64
	if _, err := fmt.Sscanf(ratio, "%f", &parsed); err != nil || parsed <= 0 || parsed > 1 {
		return sdktrace.AlwaysSample()
	}

	// ParentBased keeps a sampled trace sampled across services: a decision
	// made at the edge is honoured downstream rather than re-rolled per hop.
	return sdktrace.ParentBased(sdktrace.TraceIDRatioBased(parsed))
}

func version() string {
	if v := os.Getenv("SERVICE_VERSION"); v != "" {
		return v
	}
	return "dev"
}

func environment() string {
	if e := os.Getenv("ENVIRONMENT"); e != "" {
		return e
	}
	return "development"
}

// CorrelationID records the business identifier on a span, so a trace can be
// found from a log line and a log line from a trace.
func CorrelationID(span trace.Span, correlationID string) {
	span.SetAttributes(attribute.String("notification.correlation_id", correlationID))
}

// NotificationID records the notification a span concerns.
func NotificationID(span trace.Span, notificationID string) {
	span.SetAttributes(attribute.String("notification.id", notificationID))
}

// Channel records the delivery channel.
func Channel(span trace.Span, channel string) {
	span.SetAttributes(attribute.String("notification.channel", channel))
}
