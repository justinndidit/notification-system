package tracing

import (
	"context"

	"github.com/streadway/amqp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
)

// AMQPCarrier adapts amqp.Table to the TextMapCarrier interface so trace context
// can travel in message headers.
//
// HTTP hops are covered by instrumentation libraries, but a queue hop is not:
// producer and consumer share no connection, and without carrying the context
// explicitly a trace ends at publish and a new, unrelated one begins at
// consume — which is exactly the boundary worth being able to see across.
type AMQPCarrier amqp.Table

func (c AMQPCarrier) Get(key string) string {
	if value, ok := c[key]; ok {
		if str, ok := value.(string); ok {
			return str
		}
	}
	return ""
}

func (c AMQPCarrier) Set(key, value string) {
	c[key] = value
}

func (c AMQPCarrier) Keys() []string {
	keys := make([]string, 0, len(c))
	for k := range c {
		keys = append(keys, k)
	}
	return keys
}

// InjectAMQP writes the active trace context into message headers.
//
// Returns the headers so the caller can pass them straight to Publish. The
// table is created when nil so callers need not pre-allocate.
func InjectAMQP(ctx context.Context, headers amqp.Table) amqp.Table {
	if headers == nil {
		headers = amqp.Table{}
	}

	otel.GetTextMapPropagator().Inject(ctx, AMQPCarrier(headers))

	return headers
}

// ExtractAMQP recovers trace context from message headers, so a consumer's
// spans continue the trace that produced the message rather than starting a new
// one.
func ExtractAMQP(ctx context.Context, headers amqp.Table) context.Context {
	if headers == nil {
		return ctx
	}

	return otel.GetTextMapPropagator().Extract(ctx, AMQPCarrier(headers))
}

// Ensure the carrier satisfies the interface at compile time.
var _ propagation.TextMapCarrier = AMQPCarrier{}
