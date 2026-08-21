import { All, Controller, Req, Res } from '@nestjs/common';
import type { Request, Response } from 'express';
import { ProxyService } from './proxy.service';
import config from '../config/config';

const { templateServiceUrl } = config();

/** Proxies /template to the Template Service. Every route requires a JWT. */
@Controller('template')
export class TemplateProxyController {
  constructor(private readonly proxy: ProxyService) {}

  @All('*path')
  all(@Req() req: Request, @Res() res: Response) {
    return this.proxy.forward(req, res, templateServiceUrl!, {
      forwardUser: true,
    });
  }

  @All()
  root(@Req() req: Request, @Res() res: Response) {
    return this.proxy.forward(req, res, templateServiceUrl!, {
      forwardUser: true,
    });
  }
}
