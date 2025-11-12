import { NestFactory } from '@nestjs/core';
import { AppModule } from './app.module';
import { ValidationPipe } from '@nestjs/common';
import { HttpExceptionFilter } from './common/interceptors/response.interceptors';
import config from './config/config';
import { LoggingInterceptor } from './middleware/logging.interceptor';

import { ProxyMiddleware } from './middleware/proxy.middleware';
import { Response, NextFunction } from 'express';

const { port, userServiceUrl, orchestratorUrl, templateServiceUrl, redisUrl } =
  config();

async function bootstrap() {
  const app = await NestFactory.create(AppModule);

  app.useGlobalPipes(new ValidationPipe({ transform: true, whitelist: true }));

  app.useGlobalInterceptors(new LoggingInterceptor());

  // Error filter
  app.useGlobalFilters(new HttpExceptionFilter());
  await app.listen(port ?? 3000);

  //routes
  const proxyMiddleware = app.get(ProxyMiddleware);
  app.setGlobalPrefix('api');
  const proxyRoutes = [
    {
      path: '/user',
      target: userServiceUrl,
      requireUserHeader: (req: Request) => {
        // public routes
        const publicPaths = ['/signup', '/signin'];
        // if the request path is not public, attach user header
        return !publicPaths.includes(req.url.split('?')[0]);
      },
    },
    {
      path: '/template',
      target: templateServiceUrl,
      requireUserHeader: () => true, // all template routes require user
    },
  ];

  proxyRoutes.forEach(({ path, target, requireUserHeader }) => {
    app.use(
      path,
      (
        req: Request,
        res: Response<any, Record<string, any>>,
        next: NextFunction,
      ) => {
        proxyMiddleware.use(req, res, next);
        const addUserHeader = requireUserHeader(req);
        (req as UserRequest).proxy?.(target!, path, addUserHeader);
      },
    );
  });

  console.log(`API Gateway is running on port ${port || 3000}`);
  console.log(`User Service: ${userServiceUrl}`);
  console.log(`Orchestrator Service: ${orchestratorUrl}`);
  console.log(`Template Service: ${templateServiceUrl}`);
  console.log(`Redis: ${redisUrl}`);
}
bootstrap().catch((err) => {
  console.error('Error starting app:', err);
  process.exit(1);
});
