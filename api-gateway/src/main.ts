// Must be first: the OpenTelemetry SDK patches HTTP and framework libraries
// before anything requires them.
import './tracing';
import { NestFactory } from '@nestjs/core';
import { ValidationPipe } from '@nestjs/common';
import { AppModule } from './app.module';
import { HttpExceptionFilter } from './common/interceptors/response.interceptors';
import { LoggingInterceptor } from './middleware/logging.interceptor';
import config from './config/config';

const { port, userServiceUrl, orchestratorUrl, templateServiceUrl, corsOrigin } =
  config();

async function bootstrap() {
  const app = await NestFactory.create(AppModule, {
    logger: ['error', 'warn', 'log', 'debug', 'verbose'],
  });

  app.enableCors({
    origin: corsOrigin === '*' ? true : corsOrigin.split(',').map((o) => o.trim()),
    credentials: true,
  });

  app.useGlobalPipes(new ValidationPipe({ transform: true, whitelist: true }));
  app.useGlobalInterceptors(new LoggingInterceptor());
  app.useGlobalFilters(new HttpExceptionFilter());

  // Proxying is handled by controllers in ProxyModule rather than raw
  // app.use() middleware. Middleware registered here would run before Nest's
  // router, so the global JwtAuthGuard and ThrottlerGuard would never see
  // proxied traffic — which is exactly what used to happen.
  await app.listen(port ?? 3000);

  console.log(`\n🚀 API Gateway is running on port ${port || 3000}`);
  console.log(`📡 User Service: ${userServiceUrl}`);
  console.log(`📡 Orchestrator Service: ${orchestratorUrl}`);
  console.log(`📡 Template Service: ${templateServiceUrl}`);
  console.log(
    `\n✅ Notification endpoints available at: http://localhost:${port || 3000}/notifications\n`,
  );
}

bootstrap().catch((err) => {
  console.error('Error starting app:', err);
  process.exit(1);
});
