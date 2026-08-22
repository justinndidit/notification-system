"""
OpenTelemetry bootstrap for the email service.

Called once from the Django app's ready() hook, so it applies to all three
processes started from this image — the HTTP API, the Celery worker and the
queue bridge.

Tracing is optional: without OTEL_EXPORTER_OTLP_ENDPOINT the service runs
untraced rather than failing, so a missing collector can never stop email
delivery.
"""

import logging
import os

logger = logging.getLogger("email_service")

_initialised = False


def configure():
    """Install the tracer provider and auto-instrumentation. Idempotent."""
    global _initialised

    endpoint = os.getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
    if not endpoint or _initialised:
        return

    try:
        from opentelemetry import trace
        from opentelemetry.exporter.otlp.proto.http.trace_exporter import (
            OTLPSpanExporter,
        )
        from opentelemetry.sdk.resources import Resource
        from opentelemetry.sdk.trace import TracerProvider
        from opentelemetry.sdk.trace.export import BatchSpanProcessor
        from opentelemetry.instrumentation.celery import CeleryInstrumentor
        from opentelemetry.instrumentation.django import DjangoInstrumentor
        from opentelemetry.instrumentation.requests import RequestsInstrumentor

        provider = TracerProvider(
            resource=Resource.create(
                {
                    "service.name": "email-service",
                    "service.version": os.getenv("SERVICE_VERSION", "dev"),
                    "deployment.environment": os.getenv("ENVIRONMENT", "development"),
                }
            )
        )
        provider.add_span_processor(
            BatchSpanProcessor(OTLPSpanExporter(endpoint=f"{endpoint}/v1/traces"))
        )
        trace.set_tracer_provider(provider)

        DjangoInstrumentor().instrument()
        CeleryInstrumentor().instrument()
        # The status callback to the orchestrator goes out through requests, so
        # instrumenting it keeps the trace connected on the way back.
        RequestsInstrumentor().instrument()

        _initialised = True
        logger.info("Tracing enabled for email-service -> %s", endpoint)
    except Exception as exc:
        # Observability must never be the reason mail stops being delivered.
        logger.warning("Tracing could not be initialised: %s", exc)


def tracer():
    from opentelemetry import trace

    return trace.get_tracer("email-service")
