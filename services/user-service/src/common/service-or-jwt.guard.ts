import { Injectable, ExecutionContext } from '@nestjs/common';
import { ConfigService } from '@nestjs/config';
import { AuthGuard } from '@nestjs/passport';
import { Observable } from 'rxjs';
import { timingSafeEqual } from 'crypto';

/**
 * Allows a request through if it carries EITHER a valid user JWT OR the shared
 * internal service token.
 *
 * Used on endpoints that are called both by authenticated end users and by
 * other services during enrichment (e.g. the orchestrator reading a user's
 * delivery preferences).
 *
 * The shared secret is an interim measure. It should be replaced by real
 * service identity — mTLS, or short-lived signed service JWTs — so that a
 * single leaked value doesn't grant blanket internal access.
 */
@Injectable()
export class ServiceOrJwtGuard extends AuthGuard('jwt') {
  private readonly serviceToken: string;

  constructor(configService: ConfigService) {
    super();
    const token = configService.get<string>('INTERNAL_SERVICE_TOKEN');
    if (!token) {
      throw new Error(
        'INTERNAL_SERVICE_TOKEN is not defined in environment variables',
      );
    }
    this.serviceToken = token;
  }

  canActivate(
    context: ExecutionContext,
  ): boolean | Promise<boolean> | Observable<boolean> {
    const request = context
      .switchToHttp()
      .getRequest<{ headers: Record<string, string | string[] | undefined> }>();

    const presented = request.headers['x-service-token'];

    if (typeof presented === 'string' && this.matchesServiceToken(presented)) {
      return true;
    }

    return super.canActivate(context);
  }

  private matchesServiceToken(presented: string): boolean {
    const a = Buffer.from(presented);
    const b = Buffer.from(this.serviceToken);
    // timingSafeEqual throws on length mismatch, so compare lengths first.
    // Length is not secret; the value is.
    return a.length === b.length && timingSafeEqual(a, b);
  }
}
