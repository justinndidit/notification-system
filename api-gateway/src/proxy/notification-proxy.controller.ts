import { All, Controller, Req, Res } from '@nestjs/common';
import type { Request, Response } from 'express';
import { v4 as uuidv4 } from 'uuid';
import { ProxyService } from './proxy.service';
import config from '../config/config';

const { orchestratorUrl } = config();

const firstValue = (
  value: string | string[] | undefined,
): string | undefined => {
  if (Array.isArray(value)) {
    return value.find((v) => typeof v === 'string' && v.trim().length > 0);
  }
  return typeof value === 'string' && value.trim().length > 0
    ? value
    : undefined;
};

/**
 * Proxies /notifications to the Orchestrator.
 *
 * Attaches correlation and idempotency headers, generating them when the client
 * did not supply any, so every request is traceable and safely retryable.
 */
@Controller('notifications')
export class NotificationProxyController {
  constructor(private readonly proxy: ProxyService) {}

  private readonly headers = (req: Request) => ({
    'X-Idempotency-Key':
      firstValue(req.headers['x-idempotency-key']) ?? uuidv4(),
    'X-Correlation-ID': firstValue(req.headers['x-correlation-id']) ?? uuidv4(),
  });

  @All('*path')
  all(@Req() req: Request, @Res() res: Response) {
    return this.proxy.forward(req, res, orchestratorUrl!, {
      forwardUser: true,
      extraHeaders: this.headers,
    });
  }

  @All()
  root(@Req() req: Request, @Res() res: Response) {
    return this.proxy.forward(req, res, orchestratorUrl!, {
      forwardUser: true,
      extraHeaders: this.headers,
    });
  }
}
