/* eslint-disable @typescript-eslint/no-unsafe-call */
/* eslint-disable @typescript-eslint/no-unsafe-return */
import { Injectable, NestMiddleware, Logger } from '@nestjs/common';
import * as proxy from 'express-http-proxy';
import { Request, Response, NextFunction } from 'express';

@Injectable()
export class ProxyMiddleware implements NestMiddleware {
  private readonly logger = new Logger(ProxyMiddleware.name);

  use(req: UserRequest, res: Response, next: NextFunction) {
    req.proxy = (
      targetUrl: string,
      pathPrefix: string,
      addUserHeader = false,
    ) => {
      if (!req.url.startsWith(pathPrefix)) return;

      const proxyOptions = {
        target: targetUrl,
        changeOrigin: true,
        proxyReqPathResolver: (req: Request) =>
          req.originalUrl.replace(new RegExp(`^${pathPrefix}`), ''), // strip pathPrefix
        proxyReqOptDecorator: (
          proxyReqOpts: { headers?: Record<string, string> },
          srcReq: UserRequest,
        ) => {
          proxyReqOpts.headers = proxyReqOpts.headers || {};
          proxyReqOpts.headers['Content-Type'] = 'application/json';
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
          const message =
            err instanceof Error ? err.message : 'Unknown proxy error';
          this.logger.error(`Proxy error to ${targetUrl}: ${message}`);
          res.status(500).json({
            success: false,
            message: 'Proxy server error',
            error: message,
          });
        },
        parseReqBody: false,
      };
      if (!req.user) {
        return res
          .status(401)
          .json({ success: false, message: 'Unauthorized' });
      }

      return proxy(targetUrl, proxyOptions)(req, res, next);
    };

    next();
  }
}
