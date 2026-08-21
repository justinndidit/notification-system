import { All, Controller, Post, Req, Res } from '@nestjs/common';
import type { Request, Response } from 'express';
import { ProxyService } from './proxy.service';
import { Public } from '../auth/public.decorator';
import config from '../config/config';

const { userServiceUrl } = config();

/**
 * Proxies /user to the User Service.
 *
 * Registered as a controller rather than raw middleware so the global
 * JwtAuthGuard and ThrottlerGuard actually run. Public routes are declared with
 * @Public() on the handler itself, which replaces the previous substring test
 * against the URL — that matched any path merely *containing* "signin".
 */
@Controller('user')
export class UserProxyController {
  constructor(private readonly proxy: ProxyService) {}

  @Public()
  @Post('signup')
  signup(@Req() req: Request, @Res() res: Response) {
    return this.proxy.forward(req, res, userServiceUrl!);
  }

  @Public()
  @Post('signin')
  signin(@Req() req: Request, @Res() res: Response) {
    return this.proxy.forward(req, res, userServiceUrl!);
  }

  @All('*path')
  authenticated(@Req() req: Request, @Res() res: Response) {
    return this.proxy.forward(req, res, userServiceUrl!, { forwardUser: true });
  }

  // The wildcard above needs at least one path segment, so bare /user needs its
  // own handler or it falls through to a 404 before the guard ever runs.
  @All()
  root(@Req() req: Request, @Res() res: Response) {
    return this.proxy.forward(req, res, userServiceUrl!, { forwardUser: true });
  }
}
