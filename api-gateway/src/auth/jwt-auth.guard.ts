import { Injectable, ExecutionContext, Logger } from '@nestjs/common';
import { AuthGuard } from '@nestjs/passport';
import { Reflector } from '@nestjs/core';
import { Observable } from 'rxjs';

@Injectable()
export class JwtAuthGuard extends AuthGuard('jwt') {
  private readonly logger = new Logger(JwtAuthGuard.name);

  constructor(private reflector: Reflector) {
    super();
  }

  canActivate(
    context: ExecutionContext,
  ): boolean | Promise<boolean> | Observable<boolean> {
    const request = context.switchToHttp().getRequest<
      Request & {
        originalUrl?: string;
        url?: string;
        route?: { path?: string };
      }
    >();

    // Try multiple ways to get the route path
    let url = '';
    if (request.originalUrl) {
      url = request.originalUrl;
    } else if (request.url) {
      url = request.url;
    } else if (request.route?.path) {
      url = request.route.path;
    }

    const method = request.method || '';
    const baseUrl =
      typeof (request as unknown as { baseUrl?: string }).baseUrl === 'string'
        ? (request as unknown as { baseUrl: string }).baseUrl
        : '';

    // Public routes that don't require authentication
    const publicRoutes = ['/user/signup', '/user/signin', '/signup', '/signin'];

    // Check if the current route is public - try multiple path combinations
    const requestPath = url.split('?')[0].toLowerCase();
    const fullPath = (baseUrl + requestPath).toLowerCase();

    const isPublicRoute = publicRoutes.some((route) => {
      const normalizedRoute = route.toLowerCase();
      return (
        requestPath === normalizedRoute ||
        requestPath === normalizedRoute + '/' ||
        requestPath.endsWith('/' + normalizedRoute) ||
        requestPath.endsWith('/' + normalizedRoute + '/') ||
        fullPath === normalizedRoute ||
        fullPath === normalizedRoute + '/' ||
        fullPath.endsWith('/' + normalizedRoute) ||
        fullPath.endsWith('/' + normalizedRoute + '/') ||
        requestPath.includes('signin') ||
        requestPath.includes('signup')
      );
    });

    this.logger.log(
      `[JWT Guard] Method: ${method}, URL: ${url}, BaseURL: ${baseUrl}, RequestPath: ${requestPath}, FullPath: ${fullPath}, IsPublic: ${isPublicRoute}`,
    );

    // Allow public routes to pass through without authentication
    if (isPublicRoute) {
      this.logger.log(`✅ Allowing public route: ${method} ${requestPath}`);
      return true;
    }

    // For all other routes, require authentication
    this.logger.debug(
      `🔒 Requiring authentication for: ${method} ${requestPath}`,
    );
    return super.canActivate(context);
  }
}
