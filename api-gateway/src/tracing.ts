/**
 * OpenTelemetry bootstrap.
 *
 * Imported for its side effects as the very first line of main.ts — the SDK has
 * to patch the HTTP and framework libraries before anything requires them, so
 * ordering here is load-bearing rather than stylistic.
 *
 * Tracing is optional: without OTEL_EXPORTER_OTLP_ENDPOINT the service runs
 * untraced rather than failing, so a missing collector can never take down
 * request handling.
 */
import { NodeSDK } from '@opentelemetry/sdk-node';
import { getNodeAutoInstrumentations } from '@opentelemetry/auto-instrumentations-node';
import { OTLPTraceExporter } from '@opentelemetry/exporter-trace-otlp-http';
import { resourceFromAttributes } from '@opentelemetry/resources';
import {
  ATTR_SERVICE_NAME,
  ATTR_SERVICE_VERSION,
} from '@opentelemetry/semantic-conventions';

const endpoint = process.env.OTEL_EXPORTER_OTLP_ENDPOINT;

if (endpoint) {
  const sdk = new NodeSDK({
    resource: resourceFromAttributes({
      [ATTR_SERVICE_NAME]: 'api-gateway',
      [ATTR_SERVICE_VERSION]: process.env.SERVICE_VERSION ?? 'dev',
    }),
    traceExporter: new OTLPTraceExporter({ url: `${endpoint}/v1/traces` }),
    instrumentations: [
      getNodeAutoInstrumentations({
        // Noisy and low value: every file read would become a span.
        '@opentelemetry/instrumentation-fs': { enabled: false },
      }),
    ],
  });

  sdk.start();
  // eslint-disable-next-line no-console
  console.log(`Tracing enabled for api-gateway -> ${endpoint}`);

  const shutdown = () => {
    // Flush pending spans before exit, or the last trace is lost.
    void sdk.shutdown().finally(() => process.exit(0));
  };
  process.on('SIGTERM', shutdown);
  process.on('SIGINT', shutdown);
}
