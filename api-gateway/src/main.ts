import { NestFactory } from '@nestjs/core';
import { AppModule } from './app.module';
import { ValidationPipe } from '@nestjs/common';
import { HttpExceptionFilter } from './common/interceptors/response.interceptors';
import config from './config/config';
import { LoggingInterceptor } from './middleware/logging.interceptor';
import { ProxyMiddleware } from './middleware/proxy.middleware';
import { Response, NextFunction, Request } from 'express';
import { JwtHelper } from './common/jwt-helper';

const { port, userServiceUrl, orchestratorUrl, templateServiceUrl, redisUrl } =
  config();

async function bootstrap() {
  const app = await NestFactory.create(AppModule, {
    logger: ['error', 'warn', 'log', 'debug', 'verbose'],
  });
  // Enable CORS
  app.enableCors({
    origin: 'http://localhost:3000', // Allowed origins
    credentials: true,
  });

  app.useGlobalPipes(new ValidationPipe({ transform: true, whitelist: true }));

  app.useGlobalInterceptors(new LoggingInterceptor());

  // Error filter
  app.useGlobalFilters(new HttpExceptionFilter());

  // Set up proxy routes for user and template services
  const proxyMiddleware = app.get(ProxyMiddleware);
  const jwtHelper = app.get(JwtHelper);

  const proxyRoutes = [
    {
      path: '/user',
      target: userServiceUrl,
      requireAuth: (req: Request) => {
        // public routes that don't require authentication
        // Check both full path and path without prefix (Express strips prefix in middleware)
        const originalUrl = req.originalUrl || req.url || '';
        const url = req.url || '';
        const requestPath = originalUrl.split('?')[0];
        const relativePath = url.split('?')[0];

        const publicPaths = [
          '/user/signup',
          '/user/signin',
          '/signup',
          '/signin',
        ];

        const isPublic = publicPaths.some((publicPath) => {
          return (
            requestPath === publicPath ||
            requestPath.startsWith(publicPath + '/') ||
            relativePath === publicPath ||
            relativePath.startsWith(publicPath + '/') ||
            requestPath.includes('signin') ||
            requestPath.includes('signup')
          );
        });

        // Return true if route requires auth (i.e., NOT public)
        return !isPublic;
      },
    },
    {
      path: '/template',
      target: templateServiceUrl,
      requireAuth: () => true, // all template routes require authentication
    },
    {
      path: '/notifications',
      target: orchestratorUrl,
      requireAuth: () => false, // all orchestrator routes require authentication
    },
  ];

  // Register proxy routes
  proxyRoutes.forEach(({ path, target, requireAuth }) => {
    app.use(path, (req: Request, res: Response, next: NextFunction) => {
      // Validate JWT token and set user if present (for authenticated routes)
      const userReq = req as unknown as UserRequest;
      const token = jwtHelper.extractTokenFromRequest(req);
      //   console.log(`[JWT] Token extracted: ${token ? 'Yes' : 'No'}, Headers:`, {
      //     authorization: req.headers.authorization,
      //     Authorization: req.headers['Authorization'],
      //   });
      if (token) {
        const payload = jwtHelper.validateToken(token);
        console.log(
          `[JWT] Token validation result:`,
          payload ? 'Valid' : 'Invalid',
        );
        if (payload) {
          //   console.log(`[JWT] Setting user:`, payload);
          userReq.user = {
            userId: payload.user_id,
          };
          //   console.log(`[JWT] User set, verifying:`, userReq.user);
        } else {
          console.log(
            `[JWT] Token validation failed - token might be invalid or expired`,
          );
        }
      } else {
        console.log(`[JWT] No token found in request headers`);
      }
      proxyMiddleware.use(userReq, res, next);

      // Execute the proxy function immediately after it's set up
      if (userReq.proxy) {
        const addUserHeader = requireAuth(req);
        const originalUrl = req.originalUrl || req.url || '';
        console.log(
          `[Main] Route: ${originalUrl}, requireAuth returned: ${addUserHeader}, hasUser: ${!!userReq.user}`,
        );
        try {
          userReq.proxy(target!, path, addUserHeader);
        } catch (error) {
          const errorMessage =
            error instanceof Error ? error.message : 'Unknown error';
          console.error(`Error executing proxy: ${errorMessage}`);
          if (!res.headersSent) {
            res.status(500).json({
              success: false,
              message: 'Proxy execution error',
              error: errorMessage,
            });
          }
        }
      } else {
        // If proxy function wasn't set up, continue to next middleware
        next();
      }
    });
  });

  await app.listen(port ?? 3000);

  console.log(`\n🚀 API Gateway is running on port ${port || 3000}`);
  console.log(`📡 User Service: ${userServiceUrl}`);
  console.log(`📡 Orchestrator Service: ${orchestratorUrl}`);
  console.log(`📡 Template Service: ${templateServiceUrl}`);
  console.log(`📡 Redis: ${redisUrl}`);
  console.log(
    `\n✅ Notification endpoints available at: http://localhost:${port || 3000}/notifications\n`,
  );
}

bootstrap().catch((err) => {
  console.error('Error starting app:', err);
  process.exit(1);
});
