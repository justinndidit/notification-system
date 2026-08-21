import { Injectable, Logger } from '@nestjs/common';
import proxy from 'express-http-proxy';
import type { Request, Response } from 'express';

export type HeaderFactory = (req: Request) => Record<string, string>;

export interface ForwardOptions {
  /** Extra headers to attach, computed per request. */
  extraHeaders?: HeaderFactory;
  /** Forward the authenticated user's id as x-user-id. */
  forwardUser?: boolean;
}

/**
 * Forwards a request to an upstream service.
 *
 * The path is passed through unchanged, including the mount prefix: downstream
 * route prefixes are expected to match the gateway's, so `/user/signup` at the
 * edge is `/user/signup` at the User Service. That keeps paths stable end to
 * end and makes logs comparable across hops.
 */
@Injectable()
export class ProxyService {
  private readonly logger = new Logger(ProxyService.name);

  forward(
    req: Request,
    res: Response,
    target: string,
    options: ForwardOptions = {},
  ): Promise<void> {
    const originalUrl = req.originalUrl || req.url;

    this.logger.log(`Proxying ${req.method} ${originalUrl} -> ${target}`);

    return new Promise<void>((resolve, reject) => {
      const handler = proxy(target, {
        proxyReqPathResolver: (srcReq: Request) =>
          srcReq.originalUrl || srcReq.url || '/',

        proxyReqOptDecorator: (
          proxyReqOpts: { headers?: Record<string, string> },
          srcReq: Request,
        ) => {
          proxyReqOpts.headers = proxyReqOpts.headers ?? {};

          if (options.forwardUser) {
            const user = (srcReq as unknown as UserRequest).user;
            if (user?.userId) {
              proxyReqOpts.headers['x-user-id'] = user.userId;
            }
          }

          if (options.extraHeaders) {
            for (const [key, value] of Object.entries(
              options.extraHeaders(srcReq),
            )) {
              if (typeof value === 'string' && value.trim().length > 0) {
                proxyReqOpts.headers[key] = value;
              }
            }
          }

          return proxyReqOpts;
        },

        proxyErrorHandler: (err: unknown, errRes: Response) => {
          const message =
            err instanceof Error ? err.message : 'Unknown proxy error';
          this.logger.error(`Proxy error to ${target}: ${message}`);

          if (!errRes.headersSent) {
            errRes.status(502).json({
              success: false,
              message: 'Upstream service unavailable',
              error: message,
            });
          }
          resolve();
        },

        parseReqBody: true,
        limit: '10mb',
        timeout: 30000,
      });

      handler(req, res, (err?: unknown) => {
        if (err) {
          reject(err instanceof Error ? err : new Error(String(err)));
          return;
        }
        resolve();
      });
    });
  }
}
