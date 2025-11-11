import { NestFactory } from '@nestjs/core';
import { AppModule } from './app.module';
import {
  HttpExceptionFilter,
  ResponseInterceptor,
} from './common/interceptors/response.interceptors';
import { ValidationPipe } from '@nestjs/common';

async function bootstrap() {
  const app = await NestFactory.create(AppModule);
  app.useGlobalPipes(new ValidationPipe({ transform: true, whitelist: true }));
  app.useGlobalInterceptors(new ResponseInterceptor());
  app.useGlobalFilters(new HttpExceptionFilter());

  await app.listen(process.env.PORT ?? 3001);
}
bootstrap().catch((err) => {
  console.error('Error starting app:', err);
  process.exit(1);
});
