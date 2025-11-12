/* eslint-disable @typescript-eslint/no-unsafe-call */
/* eslint-disable @typescript-eslint/no-unsafe-return */
import { Injectable, NestMiddleware, Logger } from '@nestjs/common';
import proxy from 'express-http-proxy';
import { Request, Response, NextFunction } from 'express';

@Injectable()
export class ProxyMiddleware implements NestMiddleware {
  private readonly logger = new Logger(ProxyMiddleware.name);

  use(req: UserRequest, res: Response, next: NextFunction) {
    // Just set up the proxy function - don't call next() here
    // The caller will execute the proxy function
    req.proxy = (
      targetUrl: string,
      pathPrefix: string,
      addUserHeader = false,
    ) => {
      // Check if this request should be proxied using originalUrl
      const originalUrl = (req as unknown as Request).originalUrl || req.url;
      if (!originalUrl.startsWith(pathPrefix)) {
        return next();
      }

      // Only check auth if addUserHeader is true (route requires authentication)
      this.logger.debug(
        `[Proxy] Checking auth - addUserHeader: ${addUserHeader}, hasUser: ${!!req.user}, path: ${originalUrl}`,
      );
      if (addUserHeader && !req.user) {
        this.logger.warn(
          `[Proxy] Blocking request - requires auth but no user found: ${originalUrl}`,
        );
        return res
          .status(401)
          .json({ success: false, message: 'Unauthorized' });
      }

      this.logger.log(
        `Proxying ${req.method} ${req.url} to ${targetUrl}${originalUrl}`,
      );

      const proxyOptions = {
        target: targetUrl,
        changeOrigin: true,
        proxyReqPathResolver: (req: Request) => {
          // Keep the full path including the prefix - don't strip it
          // The user-service expects paths like /user/signin, not just /signin
          const path = req.originalUrl || req.url || '';
          this.logger.debug(`Proxying to path: ${path}`);
          return path;
        },
        proxyReqOptDecorator: (
          proxyReqOpts: { headers?: Record<string, string> },
          srcReq: UserRequest,
        ) => {
          proxyReqOpts.headers = proxyReqOpts.headers || {};
          // Don't override Content-Type if it's already set
          if (!proxyReqOpts.headers['Content-Type']) {
            proxyReqOpts.headers['Content-Type'] = 'application/json';
          }
          // Preserve all original headers
          Object.keys(srcReq.headers).forEach((key) => {
            if (!proxyReqOpts.headers![key]) {
              const headerValue: string | string[] | undefined = srcReq.headers[
                key
              ] as string | string[] | undefined;
              if (typeof headerValue === 'string') {
                proxyReqOpts.headers![key] = headerValue;
              } else if (Array.isArray(headerValue) && headerValue.length > 0) {
                const firstValue = headerValue[0];
                if (typeof firstValue === 'string') {
                  proxyReqOpts.headers![key] = firstValue;
                }
              }
            }
          });
          // Preserve original authorization header
          const authHeaderValue: string | undefined =
            typeof srcReq.headers['authorization'] === 'string'
              ? srcReq.headers['authorization']
              : typeof srcReq.headers['Authorization'] === 'string'
                ? srcReq.headers['Authorization']
                : undefined;
          if (authHeaderValue) {
            proxyReqOpts.headers['Authorization'] = authHeaderValue;
          }
          if (addUserHeader && srcReq.user) {
            proxyReqOpts.headers['x-user-id'] = srcReq.user.userId;
          }
          return proxyReqOpts;
        },
        userResDecorator: (_proxyRes: Response, proxyResData: unknown) => {
          this.logger.log(
            `Response from ${targetUrl}: ${_proxyRes.statusCode} for ${req.method ?? ''} ${req.url ?? ''}`,
          );
          return proxyResData;
        },
        proxyErrorHandler: (err: unknown, res: Response) => {
          // Check if response has already been sent
          if (res.headersSent) {
            this.logger.warn(
              'Response already sent, cannot send error response',
            );
            return;
          }
          const message =
            err instanceof Error ? err.message : 'Unknown proxy error';
          const stack = err instanceof Error ? err.stack : undefined;
          this.logger.error(`Proxy error to ${targetUrl}: ${message}`, stack);
          res.status(500).json({
            success: false,
            message: 'Proxy server error',
            error: message,
          });
        },
        parseReqBody: true,
        limit: '10mb',
        timeout: 30000, // 30 second timeout
      };

      try {
        // Execute proxy - this will handle the request and response
        // Don't pass next() to the proxy callback - the proxy handles everything
        return proxy(targetUrl, proxyOptions)(req, res, (err?: unknown) => {
          // This callback is only called if proxy fails to handle the request
          if (err) {
            const errorMessage =
              err instanceof Error
                ? err.message
                : typeof err === 'string'
                  ? err
                  : JSON.stringify(err);
            this.logger.error(`Proxy callback error: ${errorMessage}`);
            if (!res.headersSent) {
              res.status(500).json({
                success: false,
                message: 'Proxy error',
                error: errorMessage,
              });
            }
            return;
          }
          // If proxy doesn't handle it, continue to next middleware
          if (!res.headersSent) {
            next();
          }
        });
      } catch (error) {
        const errorMessage =
          error instanceof Error ? error.message : String(error);
        this.logger.error(`Error setting up proxy: ${errorMessage}`);
        if (!res.headersSent) {
          res.status(500).json({
            success: false,
            message: 'Failed to setup proxy',
            error: error instanceof Error ? error.message : 'Unknown error',
          });
        }
      }
    };
  }
}
